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
import { getDynamicPriceEntries } from '../dynamic-price'

const requestExpr =
  'len >= 500000 || c >= 20000 ? tier("500k", request(0.5)) : ' +
  'len >= 400000 || c >= 16000 ? tier("400k", request(0.4)) : ' +
  'len >= 300000 || c >= 12000 ? tier("300k", request(0.3)) : ' +
  'len >= 200000 || c >= 8000 ? tier("200k", request(0.2)) : ' +
  'tier("base", request(0.15))'

describe('dynamic per-request prices', () => {
  test('emits one request-priced entry for every tier', () => {
    const entries = parseTiersFromExpr(requestExpr).flatMap((tier) =>
      getDynamicPriceEntries(tier, { tokenUnit: 'M' })
    )

    assert.equal(entries.length, 5)
    assert.deepEqual(
      entries.map((entry) => ({ value: entry.value, unit: entry.unit })),
      [
        { value: 0.5, unit: 'request' },
        { value: 0.4, unit: 'request' },
        { value: 0.3, unit: 'request' },
        { value: 0.2, unit: 'request' },
        { value: 0.15, unit: 'request' },
      ]
    )
  })

  test('applies group and recharge conversion without token division', () => {
    const [tier] = parseTiersFromExpr('tier("base", request(0.5))')
    const [entry] = getDynamicPriceEntries(tier, {
      tokenUnit: 'K',
      groupRatioMultiplier: 2,
      showRechargePrice: true,
      priceRate: 3,
      usdExchangeRate: 6,
    })

    assert.equal(entry.unit, 'request')
    assert.equal(entry.value, 0.5)
    assert.equal(entry.formatted, '$0.5')
  })
})

describe('dynamic token price compatibility', () => {
  test('keeps token entries and selected display unit unchanged', () => {
    const [tier] = parseTiersFromExpr('tier("base", p * 3 + c * 15)')
    const entries = getDynamicPriceEntries(tier, { tokenUnit: 'K' })

    assert.deepEqual(
      entries.map((entry) => ({
        field: entry.field,
        value: entry.value,
        unit: entry.unit,
      })),
      [
        { field: 'inputPrice', value: 3, unit: 'token' },
        { field: 'outputPrice', value: 15, unit: 'token' },
      ]
    )
    assert.deepEqual(
      entries.map((entry) => entry.formatted),
      ['$0.003', '$0.015']
    )
  })
})
