package database

import "testing"

func TestBackupEvidenceAcceptsKubernetesUID(t *testing.T) {
	for _, test := range []struct {
		name  string
		uid   string
		valid bool
	}{
		{"numeric UUID", "663e1c4f-4c07-4919-a536-f56adf7bb5a4", true},
		{"alphabetic UUID", "ac3e1c4f-4c07-4919-a536-f56adf7bb5a4", true},
		{"pending UID", "", true},
		{"credential shaped", "Bearer credential", false},
		{"path", "../secret", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := BackupEvidence{State: "ready", Reason: "IsolatedRestorePending", RestoreUID: test.uid}
			if e.Valid() != test.valid {
				t.Fatal("unexpected Kubernetes UID validation")
			}
		})
	}
}
