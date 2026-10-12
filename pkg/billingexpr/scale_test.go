package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScalePriceLiterals_TieredTokenPrices(t *testing.T) {
	got, err := ScalePriceLiterals(`tier("base", p * 0.3 + c * 1.2)`, 1.5)
	require.NoError(t, err)
	assert.Equal(t, `tier("base", p * 0.45 + c * 1.8)`, got)

	got, err = ScalePriceLiterals(`tier("base", p * 0.3 + c * 1.2)`, 1.8)
	require.NoError(t, err)
	assert.Equal(t, `tier("base", p * 0.54 + c * 2.16)`, got)
}

func TestScalePriceLiterals_IdentityAndEmpty(t *testing.T) {
	got, err := ScalePriceLiterals(`tier("base", p * 0.3 + c * 1.2)`, 1)
	require.NoError(t, err)
	assert.Equal(t, `tier("base", p * 0.3 + c * 1.2)`, got)

	got, err = ScalePriceLiterals("", 1.5)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestScalePriceLiterals_FixedPrice(t *testing.T) {
	got, err := ScalePriceLiterals(`tier("base", fixed(0.02))`, 1.5)
	require.NoError(t, err)
	assert.Equal(t, `tier("base", fixed(0.03))`, got)
}

func TestScalePriceLiterals_VersionPrefix(t *testing.T) {
	got, err := ScalePriceLiterals(`v1:tier("base", p * 0.3 + c * 1.2)`, 1.5)
	require.NoError(t, err)
	assert.Equal(t, `v1:tier("base", p * 0.45 + c * 1.8)`, got)
}

func TestScalePriceLiterals_LeavesRequestMultipliers(t *testing.T) {
	// Request-rule multipliers must not be treated as $/1M prices.
	in := `tier("base", p * 0.3 + c * 1.2) * (param("stream") == true ? 1.5 : 1)`
	got, err := ScalePriceLiterals(in, 2)
	require.NoError(t, err)
	assert.Contains(t, got, `p * 0.6`)
	assert.Contains(t, got, `c * 2.4`)
	assert.Contains(t, got, `1.5`)
	assert.Contains(t, got, `1`)
}
