package gateway

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

// Deployment holds an initialized client. Credentials stay in the provider client.
type Deployment struct {
	ID       string
	Provider string
	Model    string
	Client   model.ToolCallingChatModel
}

// Registry is populated at startup and remains read-only during requests.
type Registry struct {
	deployments map[string]Deployment
}

func NewRegistry() *Registry {
	return &Registry{deployments: make(map[string]Deployment)}
}

func (r *Registry) Register(d Deployment) error {
	if strings.TrimSpace(d.ID) == "" || d.Client == nil {
		return fmt.Errorf("gateway: deployment requires an ID and client")
	}
	if r.deployments == nil {
		r.deployments = make(map[string]Deployment)
	}
	if _, exists := r.deployments[d.ID]; exists {
		return fmt.Errorf("gateway: duplicate deployment %q", d.ID)
	}
	r.deployments[d.ID] = d
	return nil
}

func (r *Registry) Get(id string) (Deployment, error) {
	d, ok := r.deployments[id]
	if !ok {
		return Deployment{}, fmt.Errorf("gateway: deployment %q not found", id)
	}
	return d, nil
}
