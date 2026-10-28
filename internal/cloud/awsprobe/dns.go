package awsprobe

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// DNSState distinguishes a foreign record from an owned but not yet converged claim.
type DNSState struct{ MarkerExists, AddressExists, Owned, Converged bool }

// DNS reads only the exact selected hosted zone and refuses foreign owners or unmarked addresses.
func DNS(ctx context.Context, configuration aws.Config, zone, hostname, marker, address string) (DNSState, error) {
	client := route53.NewFromConfig(configuration)
	zoneInfo, err := client.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: aws.String(zone)})
	if err != nil {
		return DNSState{}, err
	}
	if zoneInfo.HostedZone == nil || zoneInfo.HostedZone.Config == nil || zoneInfo.HostedZone.Config.PrivateZone {
		return DNSState{}, fmt.Errorf("approved public hosted zone is unavailable")
	}
	parent := strings.TrimSuffix(aws.ToString(zoneInfo.HostedZone.Name), ".")
	if hostname != parent && !strings.HasSuffix(hostname, "."+parent) {
		return DNSState{}, fmt.Errorf("hostname escaped observed DNS authority")
	}
	state := DNSState{}
	for _, target := range []struct {
		name string
		kind types.RRType
	}{{"_keldock." + hostname, types.RRTypeTxt}, {hostname, types.RRTypeA}} {
		response, err := client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(zone), StartRecordName: aws.String(target.name), StartRecordType: target.kind, MaxItems: aws.Int32(1)})
		if err != nil {
			return state, err
		}
		if len(response.ResourceRecordSets) == 0 {
			continue
		}
		record := response.ResourceRecordSets[0]
		if strings.TrimSuffix(aws.ToString(record.Name), ".") != target.name || record.Type != target.kind {
			continue
		}
		if target.kind == types.RRTypeTxt {
			state.MarkerExists = true
			state.Owned = len(record.ResourceRecords) == 1 && aws.ToString(record.ResourceRecords[0].Value) == `"`+marker+`"`
		} else {
			state.AddressExists = true
			state.Converged = len(record.ResourceRecords) == 1 && aws.ToString(record.ResourceRecords[0].Value) == address
		}
	}
	if (state.MarkerExists && !state.Owned) || (state.AddressExists && !state.Owned) {
		return state, fmt.Errorf("DNSOwnershipConflict")
	}
	return state, nil
}
