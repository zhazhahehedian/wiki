package domain

import "time"

type KnowledgeBase struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	OwnerID     string         `json:"owner_id"`
	EmbedModel  string         `json:"embed_model"`
	EmbedDim    int            `json:"embed_dim"`
	Settings    map[string]any `json:"settings"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
