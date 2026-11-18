package mp3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSampleRate(t *testing.T) {
	tests := []struct {
		name    string
		third   byte
		want    int
		wantErr error
	}{
		{name: "44.1 kHz", third: 0x90, want: 44100},
		{name: "48 kHz", third: 0x94, want: 48000},
		{name: "32 kHz", third: 0x98, want: 32000},
		{name: "reserved", third: 0x9c, wantErr: errIndexInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			err := p.Parse([]byte{0xff, 0xfb, tt.third})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, p.SampleRate())
		})
	}
}

func TestParseTooShort(t *testing.T) {
	require.ErrorIs(t, NewParser().Parse([]byte{0xff, 0xfb}), errMp3DataInvalid)
}

func TestDefaultSampleRate(t *testing.T) {
	assert.Equal(t, 44100, NewParser().SampleRate())
}
