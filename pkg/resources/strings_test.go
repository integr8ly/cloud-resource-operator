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

func TestStringOrDefault(t *testing.T) {
	type args struct {
		str       string
		defaultTo string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test default is returned",
			args: args{
				str:       "",
				defaultTo: "def",
			},
			want: "def",
		},
		{
			name: "test value is returned",
			args: args{
				str:       "test",
				defaultTo: "def",
			},
			want: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StringOrDefault(tt.args.str, tt.args.defaultTo); got != tt.want {
				t.Errorf("StringOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestSafeStringDereference(t *testing.T) {
	type args struct {
		str *string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test empty string is returned on nil input",
			args: args{
				str: nil,
			},
			want: "",
		},
		{
			name: "test value is returned",
			args: args{
				str: &[]string{"value"}[0],
			},
			want: "value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeStringDereference(tt.args.str); got != tt.want {
				t.Errorf("SafeStringDereference() = %v, want %v", got, tt.want)
			}
		})
	}
}
