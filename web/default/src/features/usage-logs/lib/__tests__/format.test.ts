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

import { getTieredBillingSummary } from '../format'

function encodeExpr(expr: string): string {
  return btoa(expr)
}

describe('tiered billing log prices', () => {
  test('reports request prices with a per-request unit', () => {
    const summary = getTieredBillingSummary({
      billing_mode: 'tiered_expr',
      expr_b64: encodeExpr('tier("base", request(0.15))'),
      matched_tier: 'base',
    })

    assert.notEqual(summary, null)
    assert.deepEqual(summary?.priceEntries, [
      {
        field: 'requestPrice',
        shortLabel: 'Request price',
        price: 0.15,
        unit: 'request',
      },
    ])
  })

  test('keeps token price entries marked as token units', () => {
    const summary = getTieredBillingSummary({
      billing_mode: 'tiered_expr',
      expr_b64: encodeExpr('tier("base", p * 3 + c * 15)'),
      matched_tier: 'base',
    })

    assert.deepEqual(
      summary?.priceEntries.map((entry) => ({
        field: entry.field,
        unit: entry.unit,
      })),
      [
        { field: 'inputPrice', unit: 'token' },
        { field: 'outputPrice', unit: 'token' },
      ]
    )
  })
})
