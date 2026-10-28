package bootstrap

func foundation(spec Spec) map[string]any {
	foundationName, _ := spec.Names()
	issuer := spec.IssuerBucket()
	resources := map[string]any{
		"IssuerBucket": retained("AWS::S3::Bucket", map[string]any{
			"BucketName": issuer, "OwnershipControls": ownerEnforced(), "BucketEncryption": encryption(),
			"PublicAccessBlockConfiguration": map[string]bool{"BlockPublicAcls": true, "IgnorePublicAcls": true, "BlockPublicPolicy": false, "RestrictPublicBuckets": false}, "Tags": tags(spec),
		}),
		"IssuerPolicy": retained("AWS::S3::BucketPolicy", map[string]any{
			"Bucket": map[string]string{"Ref": "IssuerBucket"}, "PolicyDocument": document([]any{
				statement("Allow", []string{"s3:GetObject"}, []string{"arn:aws:s3:::" + issuer + "/.well-known/openid-configuration", "arn:aws:s3:::" + issuer + "/keys.json"}, map[string]any{"Principal": "*", "Condition": map[string]any{"Bool": map[string]string{"aws:SecureTransport": "true"}}}),
				statement("Deny", []string{"s3:*"}, []string{"arn:aws:s3:::" + issuer, "arn:aws:s3:::" + issuer + "/*"}, map[string]any{"Principal": "*", "Condition": map[string]any{"Bool": map[string]string{"aws:SecureTransport": "false"}}}),
			}),
		}),
		"DelegatedZone": retained("AWS::Route53::HostedZone", map[string]any{"Name": spec.Domain, "HostedZoneConfig": map[string]string{"Comment": "Keldock " + spec.InstallationID + " delegated test scope"}, "HostedZoneTags": tags(spec)}),
		"ParentDelegation": retained("AWS::Route53::RecordSet", map[string]any{
			"HostedZoneId": spec.ParentZoneID, "Name": spec.Domain, "Type": "NS", "TTL": "300", "ResourceRecords": map[string]any{"Fn::GetAtt": []string{"DelegatedZone", "NameServers"}},
		}),
		"ConnectedFixtureBucket": retained("AWS::S3::Bucket", map[string]any{
			"BucketName": spec.ExistingBucket(), "OwnershipControls": ownerEnforced(), "BucketEncryption": encryption(), "VersioningConfiguration": map[string]string{"Status": "Enabled"},
			"PublicAccessBlockConfiguration": map[string]bool{"BlockPublicAcls": true, "IgnorePublicAcls": true, "BlockPublicPolicy": true, "RestrictPublicBuckets": true}, "Tags": tags(spec),
		}),
	}
	return map[string]any{
		"AWSTemplateFormatVersion": "2010-09-09", "Description": "Keldock optional AWS bootstrap: operator-owned discovery and DNS, retained on removal", "Resources": resources,
		"Outputs": map[string]any{
			"IssuerBucket":           map[string]any{"Value": map[string]string{"Ref": "IssuerBucket"}},
			"IssuerURL":              map[string]any{"Value": spec.Issuer()},
			"HostedZoneID":           map[string]any{"Value": map[string]string{"Ref": "DelegatedZone"}, "Export": map[string]string{"Name": foundationName + "-zone-id"}},
			"ConnectedFixtureBucket": map[string]any{"Value": map[string]string{"Ref": "ConnectedFixtureBucket"}},
		},
	}
}

func retained(kind string, properties map[string]any) map[string]any {
	return map[string]any{"Type": kind, "DeletionPolicy": "Retain", "UpdateReplacePolicy": "Retain", "Properties": properties}
}

func tags(spec Spec) []map[string]string {
	return []map[string]string{{"Key": "keldock:installation", "Value": spec.InstallationID}, {"Key": "keldock:purpose", "Value": "disposable-acceptance"}}
}

func ownerEnforced() map[string]any {
	return map[string]any{"Rules": []map[string]string{{"ObjectOwnership": "BucketOwnerEnforced"}}}
}
func encryption() map[string]any {
	return map[string]any{"ServerSideEncryptionConfiguration": []any{map[string]any{"ServerSideEncryptionByDefault": map[string]string{"SSEAlgorithm": "AES256"}}}}
}

func document(statements []any) map[string]any {
	return map[string]any{"Version": "2012-10-17", "Statement": statements}
}
func statement(effect string, actions []string, resources any, extra map[string]any) map[string]any {
	value := map[string]any{"Effect": effect, "Action": actions, "Resource": resources}
	for key, item := range extra {
		value[key] = item
	}
	return value
}
