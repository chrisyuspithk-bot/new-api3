/*
 * B2B2C pricing UI: tiered_expr models must display $/1M from billing_expr
 * coefficients, not from model_ratio×2.
 *
 * deepseek-v4-pro ships as:
 *   billing_expr = tier("base", p * 0.3 + c * 1.2)
 *   model_ratio  = 37.5   (shadow ratio; 37.5×2 = $75, NOT the real price)
 *
 * applyResellerPricingView used to clear billing_expr and only multiply
 * model_ratio by costFactor×markup. The card then fell back to
 * formatPrice → model_ratio×2, so:
 *   reseller (×1.5): 56.25×2 = $112.5 / $112.5   (WRONG; expect $0.45/$1.8)
 *   customer (×1.8): 67.5×2  = $135 / $135       (WRONG; expect $0.54/$2.16)
 *
 * Fix: backend keeps + scales billing_expr literals; frontend dynamic path
 * then renders the correct $/1M.
 */
import { describe, expect, it } from 'vitest'

import {
  getDynamicPricingSummary,
  isStrippedTieredExprModel,
} from '../dynamic-price'
import { formatPrice } from '../price'
import type { PricingModel } from '../../types'

const base = {
  model_name: 'deepseek-v4-pro',
  quota_type: 0,
  completion_ratio: 1,
  enable_groups: ['default'],
  group_ratio: { default: 1 },
  model_price: 0,
} as const

function model(overrides: Partial<PricingModel>): PricingModel {
  return { ...base, ...overrides } as PricingModel
}

function dynamicPair(m: PricingModel): { input?: string; output?: string } {
  const summary = getDynamicPricingSummary(m, {
    tokenUnit: 'M',
    showCurrencySymbol: false,
    groupRatioMultiplier: 1,
  })
  const find = (label: string) =>
    summary?.primaryEntries.find((e) => e.shortLabel === label)?.formatted
  return { input: find('Input'), output: find('Output') }
}

describe('B2B2C tiered_expr $/1M display', () => {
  it('admin: reads $0.3/$1.2 from billing_expr (not model_ratio×2)', () => {
    const admin = model({
      model_ratio: 37.5,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", p * 0.3 + c * 1.2)',
    })
    expect(dynamicPair(admin)).toEqual({ input: '0.3', output: '1.2' })
    // Shadow ratio×2 would wrongly show $75 — must not be used while expr exists
    expect(formatPrice(admin, 'input', 'M', false, 1, 1, 'default', false)).toBe(
      '75'
    )
  })

  it('reproduces the bug: cleared expr falls back to model_ratio×2', () => {
    const resellerBroken = model({
      model_ratio: 56.25,
      billing_mode: 'tiered_expr',
      billing_expr: '',
    })
    expect(dynamicPair(resellerBroken).input).toBeUndefined()
    expect(
      formatPrice(resellerBroken, 'input', 'M', false, 1, 1, 'default', false)
    ).toBe('112.5')
    expect(
      formatPrice(resellerBroken, 'output', 'M', false, 1, 1, 'default', false)
    ).toBe('112.5')

    const customerBroken = model({
      model_ratio: 67.5,
      billing_mode: 'tiered_expr',
      billing_expr: '',
    })
    expect(
      formatPrice(customerBroken, 'input', 'M', false, 1, 1, 'default', false)
    ).toBe('135')
  })

  it('after fix: scaled billing_expr yields costFactor×markup $/1M', () => {
    // costFactor 1.5 → 0.3*1.5 / 1.2*1.5
    expect(
      dynamicPair(
        model({
          model_ratio: 56.25,
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", p * 0.45 + c * 1.8)',
        })
      )
    ).toEqual({ input: '0.45', output: '1.8' })

    // costFactor×markup 1.8 → 0.3*1.8 / 1.2*1.8
    expect(
      dynamicPair(
        model({
          model_ratio: 67.5,
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", p * 0.54 + c * 2.16)',
        })
      )
    ).toEqual({ input: '0.54', output: '2.16' })
  })

  it('ratio/125 matches scaled input price for this shadow ratio', () => {
    // Documented coincidence: 37.5/125=0.3 because shadow ratio ≠ QuotaPerUnit*price
    expect(37.5 / 125).toBeCloseTo(0.3)
    expect(56.25 / 125).toBeCloseTo(0.45)
    expect(67.5 / 125).toBeCloseTo(0.54)
  })

  it('flags cleared tiered_expr so UI skips model_ratio×2 fallback', () => {
    expect(
      isStrippedTieredExprModel(
        model({
          model_ratio: 56.25,
          billing_mode: 'tiered_expr',
          billing_expr: '',
        })
      )
    ).toBe(true)
    expect(
      isStrippedTieredExprModel(
        model({
          model_ratio: 56.25,
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", p * 0.45 + c * 1.8)',
        })
      )
    ).toBe(false)
    expect(
      isStrippedTieredExprModel(
        model({ model_ratio: 1, billing_mode: 'ratio', billing_expr: '' })
      )
    ).toBe(false)
  })
})
