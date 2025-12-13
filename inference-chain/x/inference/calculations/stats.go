package calculations

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	zero  = decimal.NewFromInt(0)
	alpha = decimal.NewFromFloat(0.05)
)

// BinomialPValue computes the p-value for a one-sided "greater" binomial test.
// P(X >= k) where X ~ Binomial(n, p0)
// Uses iterative computation with the multiplicative formula for binomial coefficients
// to avoid factorial overflow.
func BinomialPValue(k, n int, p0 decimal.Decimal, prec int32) (decimal.Decimal, error) {
	if k < 0 || n < 0 || k > n {
		return zero, errors.New("invalid input: requires 0 <= k <= n")
	}
	if p0.LessThanOrEqual(zero) || p0.GreaterThanOrEqual(one) {
		return zero, errors.New("p0 must be in (0, 1)")
	}

	if k == 0 {
		return one, nil
	}

	q0 := one.Sub(p0)

	// Compute P(X >= k) = sum_{i=k}^{n} C(n,i) * p0^i * q0^(n-i)
	// We use the recurrence relation to compute terms efficiently:
	// P(X = i+1) / P(X = i) = (n-i)/(i+1) * p0/q0

	// Start by computing P(X = k)
	// C(n,k) * p0^k * q0^(n-k)
	prob := binomialPMF(k, n, p0, q0, prec)
	sum := prob

	// Add remaining terms P(X = k+1), P(X = k+2), ..., P(X = n)
	ratio := p0.Div(q0)
	for i := k; i < n; i++ {
		// P(X = i+1) = P(X = i) * (n-i)/(i+1) * p0/q0
		factor := decimal.NewFromInt(int64(n - i)).Div(decimal.NewFromInt(int64(i + 1)))
		prob = prob.Mul(factor).Mul(ratio)
		sum = sum.Add(prob)
	}

	return sum.Round(prec), nil
}

// binomialPMF computes P(X = k) = C(n,k) * p^k * (1-p)^(n-k)
// using the multiplicative formula for binomial coefficients.
func binomialPMF(k, n int, p, q decimal.Decimal, prec int32) decimal.Decimal {
	if k == 0 {
		return pow(q, n, prec)
	}
	if k == n {
		return pow(p, n, prec)
	}

	// Compute C(n,k) using multiplicative formula: n!/(k!(n-k)!) = product_{i=0}^{k-1} (n-i)/(i+1)
	coeff := one
	for i := 0; i < k; i++ {
		coeff = coeff.Mul(decimal.NewFromInt(int64(n - i))).Div(decimal.NewFromInt(int64(i + 1)))
	}

	// C(n,k) * p^k * q^(n-k)
	pPowK := pow(p, k, prec)
	qPowNK := pow(q, n-k, prec)

	return coeff.Mul(pPowK).Mul(qPowNK)
}

// pow computes base^exp for non-negative integer exponents.
func pow(base decimal.Decimal, exp int, prec int32) decimal.Decimal {
	if exp == 0 {
		return one
	}
	if exp == 1 {
		return base
	}

	result := one
	b := base
	for exp > 0 {
		if exp&1 == 1 {
			result = result.Mul(b)
		}
		b = b.Mul(b)
		exp >>= 1
	}
	return result.Round(prec)
}

// MissedStatTest performs a one-sided binomial test to check if the miss rate exceeds p0.
// H0: true miss rate <= p0
// Ha: true miss rate > p0
// Returns true if the test passes (H0 not rejected), false if H0 is rejected.
func MissedStatTest(nMissed, nTotal int, p0 decimal.Decimal) (bool, error) {
	if nTotal == 0 {
		return true, nil
	}
	if nMissed < 0 || nTotal < 0 || nMissed > nTotal {
		return false, errors.New("invalid input: requires 0 <= nMissed <= nTotal and nTotal > 0")
	}

	const precision int32 = 16

	pValue, err := BinomialPValue(nMissed, nTotal, p0, precision)
	if err != nil {
		return false, err
	}

	// Test passes if p-value >= alpha (cannot reject H0)
	return pValue.GreaterThanOrEqual(alpha), nil
}
