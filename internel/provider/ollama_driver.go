package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	api "github.com/cpmores/lucinda/api/v1"
	"github.com/spf13/viper"
)

type OllamaProvider struct {
	Id         string
	BaseURL    string
	HTTPClient *http.Client
}

type OllamaProviderFactory struct{}

func defaultOllamaProvider() *OllamaProvider {
	return &OllamaProvider{
		Id:      "ollama",
		BaseURL: "http://localhost:11434",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}
func (prov *OllamaProvider) GetId() string {
	return prov.Id
}
func (prov *OllamaProvider) Generate(ctx context.Context,
	req *api.ChatRequest) (*api.ChatResponse, error) {

	// 1. build ollama request
	ollamaReq := api.OllamaRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   false,
	}
	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 2. send request
	url, _ := url.JoinPath(prov.BaseURL, "/api/chat")
	httpReq, _ := http.NewRequestWithContext(ctx, "POST",
		url, bytes.NewBuffer(body))
	resp, err := prov.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 3. read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var ollamaResp api.OllamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	chatResp, err := api.ConvertOllamaToChat(ctx, ollamaResp)
	if err != nil {
		return nil, fmt.Errorf("convert response ollama to chat: %w", err)
	}

	return chatResp, nil
}

func (prov *OllamaProvider) Stream(ctx context.Context, req *api.ChatRequest) (<-chan *api.ChatResponse, error) {
	// not implement
	return nil, nil
}
func (prov *OllamaProvider) GetStatus() (*api.ProviderStatus, error) {
	// not implement
	// load to provider status
	ps := prov.NewPreProviderStatus()
	psResp, err := prov.GetProviderPsresponse()
	if err != nil {
		return nil, fmt.Errorf("get provider ps response: %w", err)
	}

	ps = AddAIModelInfoToProviderStatus(ps, psResp)
	// TODO:monitor adding TaskRuntimeStatus

	return ps, nil
}

// load taskruntime status from monitor module
func (prov *OllamaProvider) NewPreProviderStatus() *api.ProviderStatus {
	return &api.ProviderStatus{
		ID:        prov.Id,
		Timestamp: time.Now().Unix(),
		State:     1, // default healthy

		AIModelInfo: api.AIModelInfo{},
	}
}

func (prov *OllamaProvider) GetProviderPsresponse() (*api.OllamaPsResponse, error) {
	// not implement
	url, _ := url.JoinPath(prov.BaseURL, "/api/ps")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("request generation failed, url: %s", url)
	}

	resp, err := prov.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get provider ps returns code %d", resp.StatusCode)
	}

	var psResp api.OllamaPsResponse
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(respBody, &psResp); err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	return &psResp, nil
}

func (prov *OllamaProvider) CheckHealth() error {
	url, _ := url.JoinPath(prov.BaseURL, "/")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("request generation failed, url: %s", url)
	}

	resp, err := prov.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returns code %d", resp.StatusCode)
	}

	return nil
}

func (f *OllamaProviderFactory) Create(config *viper.Viper) (AIProvider, error) {
	return defaultOllamaProvider(), nil
}

func (f *OllamaProviderFactory) CreateDefault() (AIProvider, error) {
	return defaultOllamaProvider(), nil
}

func init() {
	RegisterAIProviderFactory("ollama", &OllamaProviderFactory{})
}
