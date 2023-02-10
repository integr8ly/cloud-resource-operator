package aws

import (
	"testing"
	"time"

	stratType "github.com/integr8ly/cloud-resource-operator/pkg/client/types"
)

func Test_buildAWSWindows(t *testing.T) {
	type args struct {
		timeConfig *stratType.StrategyTimeConfig
	}
	tests := []struct {
		name            string
		args            args
		backupWant      string
		maintenanceWant string
		wantErr         bool
	}{
		{
			name: "verify double digit parsing",
			args: args{
				timeConfig: stratType.NewStrategyTimeConfig(20, 00, time.Sunday, 21, 01),
			},
			backupWant:      "20:00-21:00",
			maintenanceWant: "sun:21:01-sun:22:01",
			wantErr:         false,
		},
		{
			name: "verify padding is added - expected formats hh:mm",
			args: args{
				timeConfig: stratType.NewStrategyTimeConfig(1, 05, time.Sunday, 4, 05),
			},
			backupWant:      "01:05-02:05",
			maintenanceWant: "sun:04:05-sun:05:05",
			wantErr:         false,
		},
		{
			name: "verify day laps into the following day if maintenance time is set to occur after 23hrs",
			args: args{
				timeConfig: stratType.NewStrategyTimeConfig(15, 15, time.Monday, 23, 05),
			},
			backupWant:      "15:15-16:15",
			maintenanceWant: "mon:23:05-tue:00:05",
			wantErr:         false,
		},
		{
			name: "verify there are no clashes in windows, both windows can not overlap each other, we expect an error",
			args: args{
				timeConfig: stratType.NewStrategyTimeConfig(15, 15, time.Monday, 15, 25),
			},
			backupWant:      "",
			maintenanceWant: "",
			wantErr:         true,
		},
		{
			name: "verify time overlaps by minutes, we expect an error for any overlap",
			args: args{
				timeConfig: stratType.NewStrategyTimeConfig(15, 15, time.Monday, 14, 25),
			},
			backupWant:      "",
			maintenanceWant: "",
			wantErr:         true,
		},
		{
			name: "verify formatting for maintenance day ddd:hh:mm-ddd:hh:mm",
			args: args{
				timeConfig: stratType.NewStrategyTimeConfig(15, 15, time.Monday, 16, 16),
			},
			backupWant:      "15:15-16:15",
			maintenanceWant: "mon:16:16-mon:17:16",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := buildAWSWindows(tt.args.timeConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildAWSWindows() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.backupWant {
				t.Errorf("buildAWSWindows() got = %v, want %v", got, tt.backupWant)
			}
			if got1 != tt.maintenanceWant {
				t.Errorf("buildAWSWindows() got1 = %v, want %v", got1, tt.maintenanceWant)
			}
		})
	}
}
