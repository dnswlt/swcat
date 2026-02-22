package svg

import (
	"testing"
)

func TestAdjustLightness(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		factor  float64
		want    string
		wantErr bool
	}{
		{
			name:   "brighten red",
			hex:    "#800000",
			factor: 1.5,
			want:   "#C00000",
		},
		{
			name:   "darken red",
			hex:    "#800000",
			factor: 0.5,
			want:   "#400000",
		},
		{
			name:   "no change (factor 1.0)",
			hex:    "#800000",
			factor: 1.0,
			want:   "#800000",
		},
		{
			name:   "3-digit hex",
			hex:    "#F00",
			factor: 0.5,
			want:   "#800000",
		},
		{
			name:   "no prefix",
			hex:    "800000",
			factor: 1.5,
			want:   "#C00000",
		},
		{
			name:   "clamp to white",
			hex:    "#800000",
			factor: 10.0,
			want:   "#FFFFFF",
		},
		{
			name:   "clamp to black",
			hex:    "#800000",
			factor: 0.0,
			want:   "#000000",
		},
		{
			name:    "invalid length",
			hex:     "#ABCD",
			factor:  1.0,
			wantErr: true,
		},
		{
			name:    "invalid characters",
			hex:     "#GGGGGG",
			factor:  1.0,
			wantErr: true,
		},
		{
			name:   "white stays white when brightened",
			hex:    "#FFFFFF",
			factor: 1.5,
			want:   "#FFFFFF",
		},
		{
			name:   "black stays black when brightened (0 * 1.5 = 0)",
			hex:    "#000000",
			factor: 1.5,
			want:   "#000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AdjustLightness(tt.hex, tt.factor)
			if (err != nil) != tt.wantErr {
				t.Errorf("AdjustLightness() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("AdjustLightness() = %v, want %v", got, tt.want)
			}
		})
	}
}
