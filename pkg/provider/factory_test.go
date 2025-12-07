package provider

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  time.Duration
		wantError bool
	}{
		// String inputs
		{
			name:     "string: 1s",
			input:    "1s",
			expected: time.Second,
		},
		{
			name:     "string: 500ms",
			input:    "500ms",
			expected: 500 * time.Millisecond,
		},
		{
			name:     "string: 1h30m",
			input:    "1h30m",
			expected: 90 * time.Minute,
		},
		{
			name:      "string: invalid",
			input:     "invalid",
			wantError: true,
		},

		// Duration pass-through
		{
			name:     "duration: 5s",
			input:    5 * time.Second,
			expected: 5 * time.Second,
		},

		// Integer inputs (treated as seconds)
		{
			name:     "int: 30",
			input:    int(30),
			expected: 30 * time.Second,
		},
		{
			name:     "int: 0",
			input:    int(0),
			expected: 0,
		},
		{
			name:     "int64: 60",
			input:    int64(60),
			expected: 60 * time.Second,
		},

		// Float inputs (fractional seconds)
		{
			name:     "float64: 1.5",
			input:    float64(1.5),
			expected: 1500 * time.Millisecond,
		},
		{
			name:     "float64: 0.001",
			input:    float64(0.001),
			expected: time.Millisecond,
		},

		// Error cases
		{
			name:      "nil",
			input:     nil,
			wantError: true,
		},
		{
			name:      "unsupported type: slice",
			input:     []string{},
			wantError: true,
		},
		{
			name:      "unsupported type: map",
			input:     map[string]any{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)

			if tt.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
