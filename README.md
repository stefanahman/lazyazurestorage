# lazyazurestorage

A fast, terminal-based UI for browsing and managing Azure Storage (Blob & ADLS Gen2).

## Features (Milestone 1)

*   Browse Subscriptions, Storage Accounts, Containers, and Blobs.
*   Connect to a local Azurite instance for development (`UseDevelopmentStorage=true`).
*   Authenticate via Azure CLI (`az login`).
*   **Container Management**: Create and Delete containers.
*   **Blob Management**: Upload and Delete blobs.

## How to Run

1.  Ensure you have Go installed (1.18+).
2.  Ensure you are logged in with the Azure CLI (`az login`).
3.  Run the application: `go run main.go`

## Keybindings

*   **Navigate Panes**: `tab` / `shift+tab`
*   **Navigate Lists**: `↑`/`↓` or `k`/`j`
*   **Select Item**: `enter`
*   **Create Container**: `c` (in Containers pane)
*   **Delete Item**: `d` (in Containers or Blobs pane)
*   **Upload Blob**: `u` (in Blobs pane)
*   **Toggle Help**: `?`
*   **Quit**: `q` or `ctrl+c`

## Known Limitations (v1)

*   **Tenant Switching**: The application currently operates in the tenant context of your Azure CLI login. There is no UI for switching tenants.
*   **Selection Reset**: After a create or delete operation, the selection in the list is reset to the top.
*   **AzCopy Integration**: Not yet implemented.
