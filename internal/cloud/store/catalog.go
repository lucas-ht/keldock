package store

import (
	"slices"

	"github.com/lucas-ht/keldock/internal/access"
)

// ProfileOption exposes approved product choices, without arbitrary provider/IAM inputs.
type ProfileOption struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Account         string   `json:"account"`
	Capacity        int      `json:"capacity"`
	ExistingBuckets []string `json:"existingBuckets"`
	ParentDomain    string   `json:"parentDomain,omitempty"`
}

// Catalog contains only installation-approved choices for one authorized project.
type Catalog struct {
	Accounts   []string        `json:"accounts"`
	Regions    []string        `json:"regions"`
	Identities []string        `json:"identities"`
	Profiles   []ProfileOption `json:"profiles"`
	Databases  []string        `json:"databases"`
}

// Catalog resolves static operator choices; a listed profile does not prove an AWS permission or health.
func (store *Store) Catalog(principal access.Principal, project string) (Catalog, error) {
	policy, err := store.policy(principal, project, false, false)
	if err != nil {
		return Catalog{}, err
	}
	result := Catalog{Accounts: policy.Accounts, Regions: policy.Regions, Identities: []string{}, Profiles: []ProfileOption{}, Databases: []string{}}
	for name := range policy.Identities {
		result.Identities = append(result.Identities, name)
	}
	slices.Sort(result.Identities)
	for name, profile := range policy.Profiles {
		result.Profiles = append(result.Profiles, ProfileOption{ID: name, Kind: profile.Kind, Account: profile.Account, Capacity: profile.MaxResources, ExistingBuckets: profile.ExistingBuckets, ParentDomain: profile.ParentDomain})
	}
	slices.SortFunc(result.Profiles, func(a, b ProfileOption) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for name := range policy.Databases {
		result.Databases = append(result.Databases, name)
	}
	slices.Sort(result.Databases)
	return result, nil
}
