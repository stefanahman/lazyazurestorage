package main

import (
	"context"
	"fmt"
	"lazyazurestorage/internal/azure"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	docStyle           = lipgloss.NewStyle().Margin(1, 2)
	focusedStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	blurredStyle       = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dialogBoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).BorderForeground(lipgloss.Color("205"))
	selectedItemStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	statusBarHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type pane int
const (
	subscriptionsPane pane = iota
	storageAccountsPane
	containersPane
	blobsPane
)

type uploadState int
const (
	uploadIdle uploadState = iota
	uploadGetLocalPath
	uploadGetDestPath
)

type item struct{ id, title, desc string }
func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type KeyMap struct {
	Up, Down, Tab, ShiftTab, Enter, Create, Delete, Upload, Quit, Help key.Binding
}
func (k KeyMap) ShortHelp() []key.Binding { return []key.Binding{k.Help, k.Quit} }
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Tab, k.ShiftTab, k.Enter},
		{k.Create, k.Delete, k.Upload},
		{k.Help, k.Quit},
	}
}
var keys = KeyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev pane")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Create:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create container")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Upload:   key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upload blob")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
}

type subscriptionsLoadedMsg struct{ subscriptions []list.Item; err error }
type storageAccountsLoadedMsg struct{ accounts []list.Item; err error }
type containersLoadedMsg struct{ containers []list.Item; err error }
type blobsLoadedMsg struct{ blobs []item; err error }
type containerDeletedMsg struct{ name string; err error }
type containerCreatedMsg struct{ name string; err error }
type blobDeletedMsg struct{ name string; err error }
type blobUploadedMsg struct{ name string; err error }
type clearStatusMsg struct{}

type model struct {
	azureClient          *azure.Client
	keys                 KeyMap
	help                 help.Model
	lists                []list.Model
	blobs                []item
	blobCursor           int
	spinner              spinner.Model
	textInput            textinput.Model
	loading              bool
	focused              pane
	selectedAccount      string
	selectedContainer    string
	statusMessage        string
	width, height        int
	err                  error
	confirmingDelete     bool
	confirmingDeleteBlob bool
	creatingContainer    bool
	uploadState          uploadState
	uploadLocalPath      string
	itemToDelete         item
}

func newModel(client *azure.Client) model {
	lists := make([]list.Model, 3)
	delegate := list.NewDefaultDelegate(); delegate.ShowDescription = true
	lists[subscriptionsPane] = list.New(nil, delegate, 0, 0); lists[subscriptionsPane].Title = "Subscriptions"
	lists[storageAccountsPane] = list.New(nil, delegate, 0, 0); lists[storageAccountsPane].Title = "Storage Accounts"
	lists[containersPane] = list.New(nil, delegate, 0, 0); lists[containersPane].Title = "Containers"
	s := spinner.New(); s.Spinner = spinner.Dot; s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	ti := textinput.New(); ti.Placeholder = "name"; ti.CharLimit = 256; ti.Width = 50
	h := help.New()
	return model{azureClient: client, keys: keys, help: h, lists: lists, spinner: s, textInput: ti, loading: true, focused: subscriptionsPane}
}

func (m model) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.fetchSubscriptions) }

// --- Commands ---
func (m model) fetchSubscriptions() tea.Msg {
	subs, err := m.azureClient.SubscriptionsClient.NewListPager(nil).NextPage(context.Background())
	if err != nil { return subscriptionsLoadedMsg{err: err} }
	items := []list.Item{item{id: "azurite", title: "Azurite (Local)", desc: "Connect to local Azurite emulator"}}
	for _, sub := range subs.Value {
		items = append(items, item{id: *sub.SubscriptionID, title: *sub.DisplayName, desc: *sub.SubscriptionID})
	}
	return subscriptionsLoadedMsg{subscriptions: items}
}
func (m model) fetchStorageAccounts(subscriptionID string) tea.Cmd {
	return func() tea.Msg {
		if subscriptionID == "azurite" {
			return storageAccountsLoadedMsg{accounts: []list.Item{item{id: "UseDevelopmentStorage=true", title: "Azurite", desc: "Local development storage"}}}
		}
		accountsClient, err := m.azureClient.GetAccountsClient(subscriptionID)
		if err != nil { return storageAccountsLoadedMsg{err: err} }
		pager := accountsClient.NewListPager(nil)
		accounts, err := pager.NextPage(context.Background())
		if err != nil { return storageAccountsLoadedMsg{err: err} }
		items := make([]list.Item, len(accounts.Value))
		for i, acc := range accounts.Value {
			items[i] = item{id: *acc.Name, title: *acc.Name, desc: *acc.Location}
		}
		return storageAccountsLoadedMsg{accounts: items}
	}
}
func (m model) fetchContainers(storageAccountName string) tea.Cmd {
	return func() tea.Msg {
		blobClient, err := m.azureClient.GetBlobServiceClient(storageAccountName)
		if err != nil { return containersLoadedMsg{err: err} }
		items := []list.Item{}
		pager := blobClient.NewListContainersPager(nil)
		for pager.More() {
			page, err := pager.NextPage(context.Background())
			if err != nil { return containersLoadedMsg{err: err} }
			for _, container := range page.ContainerItems {
				items = append(items, item{id: *container.Name, title: *container.Name})
			}
		}
		return containersLoadedMsg{containers: items}
	}
}
func (m model) fetchBlobs(storageAccountName, containerName string) tea.Cmd {
	return func() tea.Msg {
		blobClient, err := m.azureClient.GetBlobServiceClient(storageAccountName)
		if err != nil { return blobsLoadedMsg{err: err} }
		items := []item{}
		pager := blobClient.NewListBlobsFlatPager(containerName, nil)
		for pager.More() {
			page, err := pager.NextPage(context.Background())
			if err != nil { return blobsLoadedMsg{err: err} }
			for _, blob := range page.Segment.BlobItems {
				items = append(items, item{id: *blob.Name, title: *blob.Name})
			}
		}
		return blobsLoadedMsg{blobs: items}
	}
}
func (m model) deleteContainer(storageAccountName, containerName string) tea.Cmd {
	return func() tea.Msg {
		blobClient, err := m.azureClient.GetBlobServiceClient(storageAccountName)
		if err != nil { return containerDeletedMsg{err: err} }
		_, err = blobClient.DeleteContainer(context.Background(), containerName, nil)
		return containerDeletedMsg{name: containerName, err: err}
	}
}
func (m model) createContainer(storageAccountName, containerName string) tea.Cmd {
	return func() tea.Msg {
		blobClient, err := m.azureClient.GetBlobServiceClient(storageAccountName)
		if err != nil { return containerCreatedMsg{err: err} }
		_, err = blobClient.CreateContainer(context.Background(), containerName, nil)
		return containerCreatedMsg{name: containerName, err: err}
	}
}
func (m model) deleteBlob(storageAccountName, containerName, blobName string) tea.Cmd {
	return func() tea.Msg {
		blobClient, err := m.azureClient.GetBlobServiceClient(storageAccountName)
		if err != nil { return blobDeletedMsg{err: err} }
		_, err = blobClient.DeleteBlob(context.Background(), containerName, blobName, nil)
		return blobDeletedMsg{name: blobName, err: err}
	}
}
func (m model) uploadBlob(storageAccountName, containerName, localPath, destPath string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(localPath)
		if err != nil { return blobUploadedMsg{err: fmt.Errorf("failed to open file: %w", err)} }
		defer file.Close()
		blobClient, err := m.azureClient.GetBlobServiceClient(storageAccountName)
		if err != nil { return blobUploadedMsg{err: err} }
		_, err = blobClient.UploadFile(context.Background(), containerName, destPath, file, nil)
		return blobUploadedMsg{name: localPath, err: err}
	}
}

func (m *model) moveBlobCursor(delta int) {
	if len(m.blobs) == 0 { m.blobCursor = 0; return }
	m.blobCursor += delta
	if m.blobCursor < 0 { m.blobCursor = len(m.blobs) - 1 }
	if m.blobCursor >= len(m.blobs) { m.blobCursor = 0 }
}

func clearStatusAfter(t time.Duration) tea.Cmd {
	return tea.Tick(t, func(_ time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// --- Dialogs ---
	if m.creatingContainer || m.uploadState != uploadIdle || m.confirmingDelete || m.confirmingDeleteBlob {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
				m.creatingContainer, m.confirmingDelete, m.confirmingDeleteBlob = false, false, false
				m.uploadState = uploadIdle
				m.textInput.Reset(); return m, nil
			}
			if m.creatingContainer {
				if key.Matches(msg, m.keys.Enter) {
					m.creatingContainer = false; m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.createContainer(m.selectedAccount, m.textInput.Value()))
				}
			}
			if m.uploadState == uploadGetLocalPath {
				if key.Matches(msg, m.keys.Enter) {
					m.uploadLocalPath = m.textInput.Value()
					m.uploadState = uploadGetDestPath
					m.textInput.Placeholder = "destination/path/blob.txt"
					m.textInput.SetValue(filepath.Base(m.uploadLocalPath))
					m.textInput.CursorEnd()
					return m, nil
				}
			}
			if m.uploadState == uploadGetDestPath {
				if key.Matches(msg, m.keys.Enter) {
					destPath := m.textInput.Value()
					m.uploadState = uploadIdle
					m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.uploadBlob(m.selectedAccount, m.selectedContainer, m.uploadLocalPath, destPath))
				}
			}
			if m.confirmingDelete {
				if key.Matches(msg, key.NewBinding(key.WithKeys("y"))) {
					m.confirmingDelete = false; m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.deleteContainer(m.selectedAccount, m.itemToDelete.id))
				}
			}
			if m.confirmingDeleteBlob {
				if key.Matches(msg, key.NewBinding(key.WithKeys("y"))) {
					m.confirmingDeleteBlob = false; m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.deleteBlob(m.selectedAccount, m.selectedContainer, m.itemToDelete.id))
				}
			}
		}
		if m.creatingContainer || m.uploadState != uploadIdle {
			m.textInput, cmd = m.textInput.Update(msg); return m, cmd
		}
		return m, nil
	}

	// --- Main Update Logic ---
	// (rest of the function is identical to the last correct version)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize(); m.width, m.height = msg.Width-h, msg.Height-v
		m.help.Width = msg.Width
	case tea.KeyMsg:
		if m.loading { return m, nil }
		if key.Matches(msg, m.keys.Quit) { return m, tea.Quit }
		if key.Matches(msg, m.keys.Help) { m.help.ShowAll = !m.help.ShowAll }
		if key.Matches(msg, m.keys.Tab) { m.focused = (m.focused + 1) % 4 }
		if key.Matches(msg, m.keys.ShiftTab) { m.focused = (m.focused - 1 + 4) % 4 }

		if m.focused == blobsPane {
			switch {
			case key.Matches(msg, m.keys.Up): m.moveBlobCursor(-1)
			case key.Matches(msg, m.keys.Down): m.moveBlobCursor(1)
			case key.Matches(msg, m.keys.Delete):
				if len(m.blobs) > 0 { m.confirmingDeleteBlob = true; m.itemToDelete = m.blobs[m.blobCursor] }
			case key.Matches(msg, m.keys.Upload):
				if m.selectedContainer != "" {
					m.uploadState = uploadGetLocalPath
					m.textInput.Placeholder = "/path/to/local/file"
					m.textInput.SetValue("")
					m.textInput.Focus()
				}
			}
		} else { // It's a list pane
			switch {
			case key.Matches(msg, m.keys.Create):
				if m.focused == containersPane && m.selectedAccount != "" {
					m.creatingContainer = true; m.textInput.Placeholder = "new-container-name"; m.textInput.Focus()
				}
			case key.Matches(msg, m.keys.Delete):
				if m.focused == containersPane {
					if selectedItem, ok := m.lists[m.focused].SelectedItem().(item); ok {
						m.confirmingDelete = true; m.itemToDelete = selectedItem
					}
				}
			case key.Matches(msg, m.keys.Enter):
				if m.focused == subscriptionsPane {
					if selectedItem, ok := m.lists[m.focused].SelectedItem().(item); ok {
						m.loading = true; m.lists[storageAccountsPane].SetItems(nil); m.lists[containersPane].SetItems(nil); m.blobs = nil; m.blobCursor = 0
						cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchStorageAccounts(selectedItem.id)))
					}
				} else if m.focused == storageAccountsPane {
					if selectedItem, ok := m.lists[m.focused].SelectedItem().(item); ok {
						m.loading = true; m.selectedAccount = selectedItem.id; m.lists[containersPane].SetItems(nil); m.blobs = nil; m.blobCursor = 0
						cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchContainers(selectedItem.id)))
					}
				} else if m.focused == containersPane {
					if selectedItem, ok := m.lists[m.focused].SelectedItem().(item); ok {
						m.loading = true; m.selectedContainer = selectedItem.id; m.blobs = nil; m.blobCursor = 0
						cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchBlobs(m.selectedAccount, m.selectedContainer)))
					}
				}
			default:
				m.lists[m.focused], cmd = m.lists[m.focused].Update(msg)
				cmds = append(cmds, cmd)
			}
		}
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg); cmds = append(cmds, cmd)
	case clearStatusMsg:
		m.statusMessage, m.err = "", nil
	case subscriptionsLoadedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else { m.lists[subscriptionsPane].SetItems(msg.subscriptions) }
	case storageAccountsLoadedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else { m.lists[storageAccountsPane].SetItems(msg.accounts) }
	case containersLoadedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else { m.lists[containersPane].SetItems(msg.containers) }
	case blobsLoadedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else { m.blobs = msg.blobs }
	case containerDeletedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.statusMessage = fmt.Sprintf("Deleted container '%s'", msg.name); m.loading = true; cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchContainers(m.selectedAccount), clearStatusAfter(5*time.Second))) }
	case containerCreatedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.statusMessage = fmt.Sprintf("Created container '%s'", msg.name); m.loading = true; cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchContainers(m.selectedAccount), clearStatusAfter(5*time.Second))) }
	case blobDeletedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.statusMessage = fmt.Sprintf("Deleted blob '%s'", msg.name); m.loading = true; cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchBlobs(m.selectedAccount, m.selectedContainer), clearStatusAfter(5*time.Second))) }
	case blobUploadedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.statusMessage = fmt.Sprintf("Uploaded blob '%s'", msg.name); m.loading = true; cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchBlobs(m.selectedAccount, m.selectedContainer), clearStatusAfter(5*time.Second))) }
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 { return "Initializing..." }
	var dialog string
	if m.creatingContainer {
		inputView := fmt.Sprintf("Enter new container name:\n\n%s\n\n(enter to create, esc to cancel)", m.textInput.View())
		dialog = dialogBoxStyle.Render(inputView)
	} else if m.uploadState == uploadGetLocalPath {
		inputView := fmt.Sprintf("Enter local file path to upload:\n\n%s\n\n(enter to continue, esc to cancel)", m.textInput.View())
		dialog = dialogBoxStyle.Render(inputView)
	} else if m.uploadState == uploadGetDestPath {
		inputView := fmt.Sprintf("Enter destination blob path:\n\n%s\n\n(enter to upload, esc to cancel)", m.textInput.View())
		dialog = dialogBoxStyle.Render(inputView)
	} else if m.confirmingDelete {
		question := fmt.Sprintf("Are you sure you want to delete container '%s'? (y/n)", m.itemToDelete.title)
		dialog = dialogBoxStyle.Render(question)
	} else if m.confirmingDeleteBlob {
		question := fmt.Sprintf("Are you sure you want to delete blob '%s'? (y/n)", m.itemToDelete.title)
		dialog = dialogBoxStyle.Render(question)
	}
	if dialog != "" {
		return lipgloss.Place(m.width+docStyle.GetHorizontalFrameSize(), m.height+docStyle.GetVerticalFrameSize(), lipgloss.Center, lipgloss.Center, dialog)
	}

	mainView := m.renderBaseView()
	helpView := m.help.View(m.keys)

	var statusBar string
	if m.err != nil { statusBar = errorStyle.Render("Error: " + m.err.Error())
	} else if m.statusMessage != "" { statusBar = statusBarHelpStyle.Render(m.statusMessage) }

	return docStyle.Render(lipgloss.JoinVertical(lipgloss.Bottom, mainView, lipgloss.JoinHorizontal(lipgloss.Left, statusBar, lipgloss.NewStyle().Width(m.width-lipgloss.Width(statusBar)).Render(helpView))))
}

func (m model) renderBaseView() string {
	if m.loading && len(m.lists[subscriptionsPane].Items()) == 0 {
		return m.spinner.View() + " Loading subscriptions..."
	}

	helpView := m.help.View(m.keys)
	statusBarHeight := 1
	availableHeight := m.height - lipgloss.Height(helpView) - statusBarHeight

	sidePaneHeight := availableHeight / 3
	lastPaneHeight := availableHeight - (sidePaneHeight * 2)

	var sidePanes []string
	for i, l := range m.lists {
		style := blurredStyle
		if pane(i) == m.focused { style = focusedStyle }
		title := l.Title
		if m.loading && pane(i) == m.focused+1 { title = m.spinner.View() + " " + title }
		l.Title = title
		var paneHeight int
		if i == len(m.lists)-1 { paneHeight = lastPaneHeight } else { paneHeight = sidePaneHeight }
		sidePanes = append(sidePanes, style.Height(paneHeight).Render(l.View()))
	}
	sideBar := lipgloss.JoinVertical(lipgloss.Left, sidePanes...)

	var mainContentStr string
	if m.selectedContainer == "" {
		mainContentStr = "Select a container to see blobs."
	} else if m.loading {
		mainContentStr = m.spinner.View() + " Loading blobs..."
	} else if len(m.blobs) == 0 {
		mainContentStr = "Container is empty."
	} else {
		var b strings.Builder
		for i, blob := range m.blobs {
			if i == m.blobCursor { b.WriteString(selectedItemStyle.Render("> " + blob.title) + "\n")
			} else { b.WriteString("  " + blob.title + "\n") }
		}
		mainContentStr = b.String()
	}
	mainPaneStyle := blurredStyle
	if m.focused == blobsPane { mainPaneStyle = focusedStyle }
	mainContent := mainPaneStyle.Copy().
		Width(m.width - lipgloss.Width(sideBar)).
		Height(availableHeight).
		Render(mainContentStr)
	return lipgloss.JoinHorizontal(lipgloss.Top, sideBar, mainContent)
}

func main() {
	// f, err := tea.LogToFile("debug.log", "debug")
	// if err != nil {
	// 	fmt.Println("fatal:", err)
	// 	os.Exit(1)
	// }
	// defer f.Close()

	client, err := azure.NewClient()
	if err != nil {
		fmt.Printf("Error initializing Azure client: %v\n", err)
		os.Exit(1)
	}
	m := newModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
