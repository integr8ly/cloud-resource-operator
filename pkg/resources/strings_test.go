package resources

import "testing"

func TestShortenString(t *testing.T) {
	type args struct {
		s string
		n int
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantLen int
	}{
		{
			name: "test shorten string works with valid input",
			args: args{
				s: "my-super-long-test-name",
				n: 12,
			},
			want:    "mysuper-nm7v",
			wantLen: 12,
		},
		{
			name: "test shorten string works with invalid len input",
			args: args{
				s: "23",
				n: -1,
			},
			want:    "23-knp2",
			wantLen: 7,
		},
		{
			name: "test when len is more than string length",
			args: args{
				s: "testtest",
				n: 12,
			},
			want:    "testtest",
			wantLen: 8,
		},
		{
			name: "test hyphens are ignored in strings",
			args: args{
				s: "dimitra-qvxxg-integreatly-operator-some-other-text",
				n: 40,
			},
			want:    "dimitraqvxxgintegreatlyoperatorsome-44q7",
			wantLen: 40,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortenString(tt.args.s, tt.args.n)
			if got != tt.want {
				t.Errorf("ShortenString() = %v, want %v", got, tt.want)
			}
			if len(got) != tt.wantLen {
				t.Errorf("ShortenString() = %v, want %v", len(got), tt.wantLen)
			}
		})
	}
}
