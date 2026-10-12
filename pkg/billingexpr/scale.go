package billingexpr

import (
	"fmt"
	"math"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
)

// ScalePriceLiterals multiplies display/billing unit-price literals in an
// expression by factor. Used by B2B2C pricing views so reseller/customer
// markups show as $/1M (or per-call) instead of falling back to model_ratio*2
// after the expression is stripped.
//
// Only literals that sit opposite a billing quantity (p/c/cr/…, u("field"),
// image_count) or that are the argument of fixed()/price(), or a bare numeric
// tier body, are scaled. Request-rule multipliers are left alone.
func ScalePriceLiterals(exprStr string, factor float64) (string, error) {
	if strings.TrimSpace(exprStr) == "" {
		return exprStr, nil
	}
	if factor == 0 {
		return "", fmt.Errorf("price scale factor must be non-zero")
	}
	if factor == 1 {
		return exprStr, nil
	}
	if math.IsNaN(factor) || math.IsInf(factor, 0) {
		return "", fmt.Errorf("price scale factor must be finite")
	}

	version, body := ParseExprVersion(exprStr)
	tree, err := parser.Parse(body)
	if err != nil {
		return "", fmt.Errorf("parse billing expr: %w", err)
	}
	ast.Walk(&tree.Node, &priceLiteralScaler{factor: factor})
	out := tree.Node.String()
	if version != DefaultExprVersion {
		out = fmt.Sprintf("v%d:%s", version, out)
	} else if strings.HasPrefix(strings.TrimSpace(exprStr), "v1:") {
		out = "v1:" + out
	}
	return out, nil
}

type priceLiteralScaler struct {
	factor float64
}

func (p *priceLiteralScaler) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.BinaryNode:
		if n.Operator != "*" {
			return
		}
		if scalePriceOppositeQuantity(&n.Right, n.Left, p.factor) {
			return
		}
		scalePriceOppositeQuantity(&n.Left, n.Right, p.factor)
	case *ast.CallNode:
		callee, ok := n.Callee.(*ast.IdentifierNode)
		if !ok {
			return
		}
		switch callee.Value {
		case "fixed", "price":
			if len(n.Arguments) >= 1 {
				scaleNumericLiteral(&n.Arguments[0], p.factor)
			}
		case "tier":
			// Bare numeric tier body: tier("name", 0.02)
			if len(n.Arguments) == 2 {
				switch n.Arguments[1].(type) {
				case *ast.FloatNode, *ast.IntegerNode:
					scaleNumericLiteral(&n.Arguments[1], p.factor)
				}
			}
		}
	}
}

func scalePriceOppositeQuantity(priceSide *ast.Node, other ast.Node, factor float64) bool {
	if !isPriceQuantityNode(other) {
		return false
	}
	return scaleNumericLiteral(priceSide, factor)
}

func scaleNumericLiteral(node *ast.Node, factor float64) bool {
	switch n := (*node).(type) {
	case *ast.FloatNode:
		n.Value = scalePriceNumber(n.Value, factor)
		return true
	case *ast.IntegerNode:
		*node = &ast.FloatNode{Value: scalePriceNumber(float64(n.Value), factor)}
		return true
	}
	return false
}

func scalePriceNumber(value, factor float64) float64 {
	scaled, _ := decimal.NewFromFloat(value).Mul(decimal.NewFromFloat(factor)).Round(10).Float64()
	return math.Round(scaled*1e10) / 1e10
}

func isPriceQuantityNode(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.IdentifierNode:
		switch n.Value {
		case "p", "c", "cr", "cc", "cc_1h", "img", "img_cr", "img_o", "ai", "ao", "len", "image_count":
			return true
		}
	case *ast.CallNode:
		callee, ok := n.Callee.(*ast.IdentifierNode)
		return ok && callee.Value == "u"
	}
	return false
}
