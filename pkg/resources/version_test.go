package resources

import (
	"reflect"
	"testing"
)

func Test_VerifyVersionUpgradeNeeded(t *testing.T) {

	type test struct {
		name    string
		current string
		desired string
		wantErr string
		want    bool
	}

	tests := []test{
		{
			name:    "upgrade not needed when versions are the same",
			current: "10.1",
			desired: "10.1",
			want:    false,
		},
		{
			name:    "upgrade not needed when current is higher than desired",
			current: "10.2",
			desired: "10.1",
			want:    false,
		},
		{
			name:    "upgrade needed when current is lower than desired",
			current: "10.1",
			desired: "11.1",
			want:    true,
		},
		{
			name:    "error when current is invalid",
			current: "some broken value",
			desired: "11.1",
			want:    false,
			wantErr: "failed to parse current version: Malformed version: some broken value",
		},
		{
			name:    "error when desired is invalid",
			current: "10.1",
			desired: "some broken value",
			want:    false,
			wantErr: "failed to parse desired version: Malformed version: some broken value",
		},
	}

	for _, tt := range tests {
		got, err := VerifyVersionUpgradeNeeded(tt.current, tt.desired)

		if err != nil {
			if tt.wantErr == "" {
				t.Errorf("VerifyVersionUpgradedNeeded() error: %v", err)
			} else if tt.wantErr != "" && err.Error() != tt.wantErr {
				t.Errorf("VerifyVersionUpgradedNeeded() wanted error %v, got error %v", tt.wantErr, err.Error())
			}
		}

		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("buildRDSUpdateStrategy() = %v, want %v", got, tt.want)
		}
	}
}
