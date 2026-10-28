package cloud

import (
	"fmt"
	"time"

	"github.com/lucas-ht/keldock/internal/provenance"
)

// AdoptionGrant is an operator-supplied release record, never accepted from application/API input.
// The operator must remove/freeze the previous writer before recording its observed inventory hash.
type AdoptionGrant struct {
	InventoryHash string    `json:"inventoryHash"`
	ReleaseRef    string    `json:"releaseRef"`
	ReleasedAt    time.Time `json:"releasedAt"`
}

// AdoptionReview binds an administrator's confirmation to exact observed identity and writer release.
func AdoptionReview(resource Resource, connection Connection, inventoryHash string, grant AdoptionGrant) (string, error) {
	if resource.Mode != "connect" || resource.Owner != "external" || resource.ConnectionGeneration != connection.Generation || inventoryHash != grant.InventoryHash || grant.ReleaseRef == "" {
		return "", fmt.Errorf("adoption lacks current identity and released-writer authority")
	}
	return provenance.Hash(struct {
		ResourceID                             string
		Generation                             int64
		ConnectionID                           string
		ConnectionGeneration                   int64
		Account, Region, Bucket, InventoryHash string
		Grant                                  AdoptionGrant
	}{resource.ID, resource.Generation, connection.ID, connection.Generation, connection.Account, connection.Region, resource.ExternalName, inventoryHash, grant})
}
