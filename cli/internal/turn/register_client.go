package turn

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type RegisterResponse struct {
	TurnUUID   string `json:"turn_uuid"`
	VisitURL   string `json:"visit_url"`
	AgentWSURL string `json:"agent_ws_url"`
}

type RegisterClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewRegisterClient(baseURL string) *RegisterClient {
	return &RegisterClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func (c *RegisterClient) Register() (RegisterResponse, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/register", bytes.NewBuffer(nil))
	if err != nil {
		return RegisterResponse{}, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return RegisterResponse{}, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return RegisterResponse{}, fmt.Errorf("register failed with status: %d", res.StatusCode)
	}

	var out RegisterResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return RegisterResponse{}, err
	}
	return out, nil
}
