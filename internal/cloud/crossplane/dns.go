package crossplane

import (
	"fmt"

	"github.com/lucas-ht/keldock/internal/cloud"
)

// DNSMarker is unique to the project and claim, and survives resource-controller interruption.
func DNSMarker(claim cloud.DomainClaim) string {
	return "keldock:" + claim.OrganizationID + ":" + claim.ProjectID + ":" + claim.ID
}

// DNSRecords owns only a marker TXT and one A record in an operator-owned child zone.
func DNSRecords(policy cloud.Policy, connection cloud.Connection, claim cloud.DomainClaim) (map[string]any, map[string]any, error) {
	if err := policy.ValidateDomain(connection, claim.Request); err != nil {
		return nil, nil, err
	}
	if claim.Scope != policy.Scope || claim.ConnectionGeneration != connection.Generation {
		return nil, nil, fmt.Errorf("domain projection outside current connection")
	}
	profile := policy.Profiles[claim.Request.Profile]
	makeRecord := func(id, name, kind, value string) map[string]any {
		parameters := map[string]any{"zoneId": profile.HostedZoneID, "name": name, "type": kind, "ttl": 60, "records": []string{value}, "allowOverwrite": false}
		record := managed(policy, connection, id, "Record", "route53.aws.m.upbound.io", "route53", profile.HostedZoneID+"_"+name+"_"+kind, parameters, false)
		// DNS records are disposable only after an ownership check by the cloud agent.
		record["spec"].(map[string]any)["managementPolicies"] = []string{"Observe", "Create", "Update", "Delete"}
		return record
	}
	return makeRecord(claim.ID+"-marker", "_keldock."+claim.Request.Hostname, "TXT", DNSMarker(claim)), makeRecord(claim.ID+"-address", claim.Request.Hostname, "A", profile.IngressIPv4), nil
}
