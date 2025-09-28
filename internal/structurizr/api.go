package structurizr

import "strings"

type Element interface {
	GetID() string
	GetName() string
	GetKind() string
	GetTags() []string
}

type Relationship struct {
	Description          string `json:"description"`
	SourceID             string `json:"sourceId"`
	DestinationID        string `json:"destinationId"`
	LinkedRelationshipID string `json:"linkedRelationshipId"`
	Technology           string `json:"technology"`
	Tags                 string `json:"tags"`
}

type Component struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Relationships []*Relationship `json:"relationships"`
	Tags          string          `json:"tags"`
}

func (c *Component) GetID() string {
	return c.ID
}
func (c *Component) GetName() string {
	return c.Name
}
func (c *Component) GetKind() string {
	return "Component"
}
func (c *Component) GetTags() []string {
	return strings.Split(c.Tags, ",")
}

type Container struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Group         string            `json:"group"`
	Components    []*Component      `json:"components"`
	Relationships []*Relationship   `json:"relationships"`
	Properties    map[string]string `json:"properties"`
	Tags          string            `json:"tags"`
}

func (c *Container) GetID() string {
	return c.ID
}
func (c *Container) GetName() string {
	return c.Name
}
func (c *Container) GetKind() string {
	return "Container"
}
func (c *Container) GetTags() []string {
	return strings.Split(c.Tags, ",")
}

type SoftwareSystem struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Containers    []*Container      `json:"containers"`
	Relationships []*Relationship   `json:"relationships"`
	Properties    map[string]string `json:"properties"`
	Tags          string            `json:"tags"`
}

func (c *SoftwareSystem) GetID() string {
	return c.ID
}
func (c *SoftwareSystem) GetName() string {
	return c.Name
}
func (c *SoftwareSystem) GetKind() string {
	return "SoftwareSystem"
}
func (c *SoftwareSystem) GetTags() []string {
	return strings.Split(c.Tags, ",")
}

type Model struct {
	SoftwareSystems []*SoftwareSystem `json:"softwareSystems"`
}

type Workspace struct {
	Model *Model `json:"model"`
}
