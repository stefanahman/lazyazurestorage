package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

const (
	// Standard Azurite connection string for "UseDevelopmentStorage=true"
	azuriteConnectionString = "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
)

// Client manages interactions with Azure APIs.
type Client struct {
	cred                azcore.TokenCredential
	SubscriptionsClient *armsubscriptions.Client
}

// NewClient creates a new client for interacting with Azure.
func NewClient() (*Client, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create default credential: %w", err)
	}

	subsClient, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	return &Client{
		cred:                cred,
		SubscriptionsClient: subsClient,
	}, nil
}

// GetAccountsClient creates a new AccountsClient for a specific subscription.
func (c *Client) GetAccountsClient(subscriptionID string) (*armstorage.AccountsClient, error) {
	accountsClient, err := armstorage.NewAccountsClient(subscriptionID, c.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create accounts client: %w", err)
	}
	return accountsClient, nil
}

// GetBlobServiceClient creates a client for a specific storage account's blob service.
// It supports connecting to Azurite via the "UseDevelopmentStorage=true" shortcut.
func (c *Client) GetBlobServiceClient(storageAccountName string) (*azblob.Client, error) {
	if storageAccountName == "UseDevelopmentStorage=true" {
		serviceClient, err := azblob.NewClientFromConnectionString(azuriteConnectionString, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create blob service client from Azurite connection string: %w", err)
		}
		return serviceClient, nil
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccountName)
	serviceClient, err := azblob.NewClient(serviceURL, c.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob service client for account %s: %w", storageAccountName, err)
	}

	return serviceClient, nil
}
