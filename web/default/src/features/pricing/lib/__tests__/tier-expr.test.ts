/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { parseTiersFromExpr } from '../billing-expr'
import {
  CACHE_MODE_GENERIC,
  evalExprLocally,
  generateExprFromVisualConfig,
  setVisualPricingUnit,
  tryParseVisualConfig,
  type ExtraTokenValues,
  type VisualConfig,
} from '../tier-expr'

const emptyExtraTokens: ExtraTokenValues = {
  cacheReadTokens: 0,
  cacheCreateTokens: 0,
  cacheCreate1hTokens: 0,
  imageTokens: 0,
  imageOutputTokens: 0,
  audioInputTokens: 0,
  audioOutputTokens: 0,
}

const requestConfig: VisualConfig = {
  pricing_unit: 'request',
  tiers: [
    {
      label: '500k',
      condition_logic: 'or',
      conditions: [
        { var: 'len', op: '>=', value: 500000 },
        { var: 'c', op: '>=', value: 20000 },
      ],
      request_price: 0.5,
      input_unit_cost: 0,
      output_unit_cost: 0,
      cache_mode: CACHE_MODE_GENERIC,
    },
    {
      label: 'base',
      conditions: [],
      request_price: 0.15,
      input_unit_cost: 0,
      output_unit_cost: 0,
      cache_mode: CACHE_MODE_GENERIC,
    },
  ],
}

describe('per-request visual tiers', () => {
  test('generates request prices and OR conditions', () => {
    assert.equal(
      generateExprFromVisualConfig(requestConfig),
      'len >= 500000 || c >= 20000 ? tier("500k", request(0.5)) : tier("base", request(0.15))'
    )
  })

  test('round-trips request prices and condition logic', () => {
    const expr = generateExprFromVisualConfig(requestConfig)
    const parsed = tryParseVisualConfig(expr)

    assert.notEqual(parsed, null)
    assert.equal(parsed?.pricing_unit, 'request')
    assert.equal(parsed?.tiers[0].condition_logic, 'or')
    assert.deepEqual(parsed?.tiers[0].conditions, [
      { var: 'len', op: '>=', value: 500000 },
      { var: 'c', op: '>=', value: 20000 },
    ])
    assert.equal(parsed?.tiers[0].request_price, 0.5)
    assert.equal(parsed?.tiers[1].request_price, 0.15)
    assert.equal(generateExprFromVisualConfig(parsed), expr)
  })

  test('parses nested request calls without truncating the tier body', () => {
    const tiers = parseTiersFromExpr(
      'len >= 500000 || c >= 20000 ? tier("500k", request(0.5)) : tier("base", request(0.15))'
    )

    assert.equal(tiers.length, 2)
    assert.equal(tiers[0].label, '500k')
    assert.equal(tiers[0].conditionLogic, 'or')
    assert.equal(tiers[0].requestPrice, 0.5)
    assert.equal(tiers[1].label, 'base')
    assert.equal(tiers[1].requestPrice, 0.15)
  })

  test('evaluates one request price on the backend-compatible scale', () => {
    const result = evalExprLocally(
      'tier("base", request(0.15))',
      900000,
      900000,
      emptyExtraTokens
    )

    assert.equal(result.error, null)
    assert.equal(result.cost, 150000)
    assert.equal(result.matchedTier, 'base')
  })
})

describe('visual tier compatibility', () => {
  test('switches pricing units without losing either price configuration', () => {
    const tokenConfig: VisualConfig = {
      pricing_unit: 'token',
      tiers: [
        {
          label: 'base',
          conditions: [],
          input_unit_cost: 3,
          output_unit_cost: 15,
          cache_mode: CACHE_MODE_GENERIC,
        },
      ],
    }

    const requestMode = setVisualPricingUnit(tokenConfig, 'request')
    assert.equal(requestMode.pricing_unit, 'request')
    assert.equal(requestMode.tiers[0].request_price, 0)
    assert.equal(requestMode.tiers[0].input_unit_cost, 3)
    assert.equal(requestMode.tiers[0].output_unit_cost, 15)

    const tokenMode = setVisualPricingUnit(
      {
        ...requestMode,
        tiers: [{ ...requestMode.tiers[0], request_price: 0.15 }],
      },
      'token'
    )
    assert.equal(tokenMode.pricing_unit, 'token')
    assert.equal(tokenMode.tiers[0].request_price, 0.15)
    assert.equal(tokenMode.tiers[0].input_unit_cost, 3)
    assert.equal(tokenMode.tiers[0].output_unit_cost, 15)
  })

  test('keeps existing token tier generation unchanged', () => {
    const config: VisualConfig = {
      pricing_unit: 'token',
      tiers: [
        {
          label: 'base',
          conditions: [],
          input_unit_cost: 3,
          output_unit_cost: 15,
          cache_mode: CACHE_MODE_GENERIC,
        },
      ],
    }

    assert.equal(
      generateExprFromVisualConfig(config),
      'tier("base", p * 3 + c * 15)'
    )
  })

  test('keeps homogeneous AND conditions', () => {
    const config: VisualConfig = {
      pricing_unit: 'request',
      tiers: [
        {
          ...requestConfig.tiers[0],
          condition_logic: 'and',
        },
        requestConfig.tiers[1],
      ],
    }

    const expr = generateExprFromVisualConfig(config)
    assert.equal(
      expr,
      'len >= 500000 && c >= 20000 ? tier("500k", request(0.5)) : tier("base", request(0.15))'
    )
    assert.equal(tryParseVisualConfig(expr)?.tiers[0].condition_logic, 'and')
  })

  test('keeps mixed boolean conditions in raw mode', () => {
    const expr =
      'len >= 500000 && c >= 20000 || p >= 1000 ? tier("mixed", request(0.5)) : tier("base", request(0.15))'

    assert.equal(tryParseVisualConfig(expr), null)
  })

  test('keeps mixed request and token prices in raw mode', () => {
    const expr =
      'len >= 500000 ? tier("request", request(0.5)) : tier("token", p * 3 + c * 15)'

    assert.equal(tryParseVisualConfig(expr), null)
  })
})
