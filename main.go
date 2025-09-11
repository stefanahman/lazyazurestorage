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
	navPane pane = iota
	containerPane
	blobPane
)

type navState int
const (
	navStateSubscription navState = iota
	navStateStorageAccount
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
	Up, Down, Tab, ShiftTab, Enter, Back, Create, Delete, Upload, Quit, Help key.Binding
}
func (k KeyMap) ShortHelp() []key.Binding { return []key.Binding{k.Help, k.Quit, k.Back} }
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Tab, k.ShiftTab, k.Enter, k.Back},
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
	Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
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
	navPane              list.Model
	containerPane        list.Model
	navState             navState
	blobs                []item
	blobCursor           int
	spinner              spinner.Model
	textInput            textinput.Model
	loading              bool
	focused              pane
	selectedSubscription item
	selectedAccount      item
	selectedContainer    item
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
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	nav := list.New(nil, delegate, 0, 0); nav.Title = "Subscriptions"
	containers := list.New(nil, delegate, 0, 0); containers.Title = "Containers"
	s := spinner.New(); s.Spinner = spinner.Dot; s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	ti := textinput.New(); ti.Placeholder = "name"; ti.CharLimit = 256; ti.Width = 50
	h := help.New()
	return model{azureClient: client, keys: keys, help: h, navPane: nav, containerPane: containers, navState: navStateSubscription, spinner: s, textInput: ti, loading: true, focused: navPane}
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

// --- UPDATE ---
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// --- Dialogs ---
	if m.creatingContainer || m.uploadState != uploadIdle || m.confirmingDelete || m.confirmingDeleteBlob {
		if msg, ok := msg.(tea.KeyMsg); ok {
			if key.Matches(msg, m.keys.Back) {
				m.creatingContainer, m.confirmingDelete, m.confirmingDeleteBlob = false, false, false
				m.uploadState = uploadIdle
				m.textInput.Reset(); return m, nil
			}
		}

		if m.creatingContainer {
			if msg, ok := msg.(tea.KeyMsg); ok {
				if key.Matches(msg, m.keys.Enter) {
					m.creatingContainer = false; m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.createContainer(m.selectedAccount.id, m.textInput.Value()))
				}
			}
			m.textInput, cmd = m.textInput.Update(msg); return m, cmd
		}
		if m.uploadState != uploadIdle {
			if msg, ok := msg.(tea.KeyMsg); ok {
				if key.Matches(msg, m.keys.Enter) {
					if m.uploadState == uploadGetLocalPath {
						m.uploadLocalPath = m.textInput.Value()
						m.uploadState = uploadGetDestPath
						m.textInput.Placeholder = "destination/path/blob.txt"
						m.textInput.SetValue(filepath.Base(m.uploadLocalPath))
						m.textInput.CursorEnd()
						return m, nil
					} else { // uploadGetDestPath
						destPath := m.textInput.Value()
						m.uploadState = uploadIdle
						m.loading = true
						return m, tea.Batch(m.spinner.Tick, m.uploadBlob(m.selectedAccount.id, m.selectedContainer.id, m.uploadLocalPath, destPath))
					}
				}
			}
			m.textInput, cmd = m.textInput.Update(msg); return m, cmd
		}
		if m.confirmingDelete {
			if msg, ok := msg.(tea.KeyMsg); ok {
				if key.Matches(msg, key.NewBinding(key.WithKeys("y"))) {
					m.confirmingDelete = false; m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.deleteContainer(m.selectedAccount.id, m.itemToDelete.id))
				}
			}
		}
		if m.confirmingDeleteBlob {
			if msg, ok := msg.(tea.KeyMsg); ok {
				if key.Matches(msg, key.NewBinding(key.WithKeys("y"))) {
					m.confirmingDeleteBlob = false; m.loading = true
					return m, tea.Batch(m.spinner.Tick, m.deleteBlob(m.selectedAccount.id, m.selectedContainer.id, m.itemToDelete.id))
				}
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize(); m.width, m.height = msg.Width-h, msg.Height-v
		m.help.Width = msg.Width
		sidePaneWidth := m.width / 3
		helpHeight := lipgloss.Height(m.help.View(m.keys))
		mainViewHeight := m.height - helpHeight

		m.navPane.SetSize(sidePaneWidth, (mainViewHeight/2)-1)
		m.containerPane.SetSize(sidePaneWidth, mainViewHeight-(mainViewHeight/2)-1)
	case tea.KeyMsg:
		if m.loading { return m, nil }
		if key.Matches(msg, m.keys.Quit) { return m, tea.Quit }
		if key.Matches(msg, m.keys.Help) { m.help.ShowAll = !m.help.ShowAll }
		if key.Matches(msg, m.keys.Tab) { m.focused = (m.focused + 1) % 3 }
		if key.Matches(msg, m.keys.ShiftTab) { m.focused = (m.focused - 1 + 3) % 3 }
		if key.Matches(msg, m.keys.Back) {
			if m.navState == navStateStorageAccount {
				m.navState = navStateSubscription
				m.loading = true
				cmds = append(cmds, m.fetchSubscriptions)
			}
		}

		if m.focused == navPane {
			if key.Matches(msg, m.keys.Enter) {
				selected, ok := m.navPane.SelectedItem().(item)
				if ok {
					if m.navState == navStateSubscription {
						m.selectedSubscription = selected
						m.navState = navStateStorageAccount
						m.loading = true
						cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchStorageAccounts(selected.id)))
					} else { // navStateStorageAccount
						m.selectedAccount = selected
						m.loading = true
						cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchContainers(selected.id)))
					}
				}
			}
		} else if m.focused == containerPane {
			if key.Matches(msg, m.keys.Enter) {
				selected, ok := m.containerPane.SelectedItem().(item)
				if ok {
					m.selectedContainer = selected
					m.loading = true
					cmds = append(cmds, tea.Batch(m.spinner.Tick, m.fetchBlobs(m.selectedAccount.id, selected.id)))
				}
			} else if key.Matches(msg, m.keys.Create) {
				m.creatingContainer = true; m.textInput.Placeholder = "new-container-name"; m.textInput.Focus()
			} else if key.Matches(msg, m.keys.Delete) {
				if selected, ok := m.containerPane.SelectedItem().(item); ok {
					m.confirmingDelete = true; m.itemToDelete = selected
				}
			}
		} else if m.focused == blobPane {
			switch {
			case key.Matches(msg, m.keys.Up): m.moveBlobCursor(-1)
			case key.Matches(msg, m.keys.Down): m.moveBlobCursor(1)
			case key.Matches(msg, m.keys.Delete):
				if len(m.blobs) > 0 { m.confirmingDeleteBlob = true; m.itemToDelete = m.blobs[m.blobCursor] }
			case key.Matches(msg, m.keys.Upload):
				m.uploadState = uploadGetLocalPath; m.textInput.Placeholder = "/path/to/local/file"; m.textInput.SetValue(""); m.textInput.Focus()
			}
		}
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg); cmds = append(cmds, cmd)
	case clearStatusMsg:
		m.statusMessage, m.err = "", nil
	case subscriptionsLoadedMsg:
		m.loading = false
		if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.navPane.SetItems(msg.subscriptions); m.navPane.Title = "Subscriptions"
		}
	case storageAccountsLoadedMsg:
		m.loading = false
		if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.navPane.SetItems(msg.accounts); m.navPane.Title = fmt.Sprintf("Accounts in %s", m.selectedSubscription.title)
		}
	case containersLoadedMsg:
		m.loading = false
		if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.containerPane.SetItems(msg.containers)
		}
	case blobsLoadedMsg:
		m.loading = false; if msg.err != nil { m.err = msg.err; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else { m.blobs = msg.blobs }
	case containerDeletedMsg, containerCreatedMsg, blobDeletedMsg, blobUploadedMsg:
		m.loading = false
		var anErr error; var status string; var refreshCmd tea.Cmd
		switch msg := msg.(type) {
		case containerDeletedMsg: anErr = msg.err; status = fmt.Sprintf("Deleted container '%s'", msg.name); refreshCmd = m.fetchContainers(m.selectedAccount.id)
		case containerCreatedMsg: anErr = msg.err; status = fmt.Sprintf("Created container '%s'", msg.name); refreshCmd = m.fetchContainers(m.selectedAccount.id)
		case blobDeletedMsg: anErr = msg.err; status = fmt.Sprintf("Deleted blob '%s'", msg.name); refreshCmd = m.fetchBlobs(m.selectedAccount.id, m.selectedContainer.id)
		case blobUploadedMsg: anErr = msg.err; status = fmt.Sprintf("Uploaded blob '%s'", msg.name); refreshCmd = m.fetchBlobs(m.selectedAccount.id, m.selectedContainer.id)
		}
		if anErr != nil { m.err = anErr; cmds = append(cmds, clearStatusAfter(5*time.Second)) } else {
			m.statusMessage = status; m.loading = true; cmds = append(cmds, tea.Batch(m.spinner.Tick, refreshCmd, clearStatusAfter(5*time.Second))) }
	}

	if m.focused == navPane {
		m.navPane, cmd = m.navPane.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.focused == containerPane {
		m.containerPane, cmd = m.containerPane.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// --- VIEW ---
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
	statusBar := m.renderStatusBar()

	return docStyle.Render(lipgloss.JoinVertical(lipgloss.Bottom, mainView, lipgloss.JoinHorizontal(lipgloss.Left, statusBar, lipgloss.NewStyle().Width(m.width-lipgloss.Width(statusBar)).Render(helpView))))
}

func (m model) renderStatusBar() string {
	if m.err != nil { return errorStyle.Render("Error: " + m.err.Error()) }
	if m.statusMessage != "" { return statusBarHelpStyle.Render(m.statusMessage) }
	return ""
}

func (m model) renderBaseView() string {
	if m.loading && m.navPane.Items() == nil {
		return m.spinner.View() + " Loading subscriptions..."
	}

	helpView := m.help.View(m.keys)
	statusBarHeight := 1
	availableHeight := m.height - lipgloss.Height(helpView) - statusBarHeight
	sidePaneWidth := m.width / 3

	navStyle := blurredStyle
	if m.focused == navPane { navStyle = focusedStyle }
	containerStyle := blurredStyle
	if m.focused == containerPane { containerStyle = focusedStyle }

	sidebar := lipgloss.JoinVertical(lipgloss.Left,
		navStyle.Height(availableHeight/2).Width(sidePaneWidth).Render(m.navPane.View()),
		containerStyle.Height(availableHeight-(availableHeight/2)).Width(sidePaneWidth).Render(m.containerPane.View()),
	)

	var mainContentStr string
	if m.selectedAccount.id == "" {
		mainContentStr = "Select a storage account."
	} else if m.selectedContainer.id == "" {
		mainContentStr = "Select a container to see blobs."
	} else if m.loading {
		mainContentStr = m.spinner.View() + " Loading..."
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
	blobStyle := blurredStyle
	if m.focused == blobPane { blobStyle = focusedStyle }
	mainContent := blobStyle.Width(m.width - sidePaneWidth).Height(availableHeight).Render(mainContentStr)

	return lipgloss.JoinHorizontal(lipgloss.Left, sidebar, mainContent)
}

func main() {
	client, err := azure.NewClient()
	if err != nil { fmt.Printf("Error initializing Azure client: %v\n", err); os.Exit(1) }
	m := newModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil { fmt.Println("Error running program:", err); os.Exit(1) }
}
