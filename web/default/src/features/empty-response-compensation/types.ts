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
export type EmptyResponseCompensationStatus =
  | 'pending'
  | 'claimed'
  | 'blocked'
  | 'expired'

export type EmptyResponseCompensationRecord = {
  id: number
  user_id: number
  username?: string
  request_id: string
  model_name: string
  channel_id: number
  token_id: number
  prompt_tokens: number
  completion_tokens: number
  original_quota: number
  compensation_ratio: number
  compensation_quota: number
  status: EmptyResponseCompensationStatus
  block_reason: string
  detection_reason: string
  is_stream: boolean
  created_at: number
  expires_at: number
  claimed_at: number
}

export type EmptyResponseCompensationRules = {
  min_qualification_amount: number
  input_token_threshold: number
  output_token_threshold: number
  claim_window_days: number
  daily_claim_limit: number
  announcement: string
}

export type EmptyResponseCompensationSummary = {
  qualification_amount: number
  qualified: boolean
  pending_count: number
  pending_quota: number
  claimed_count: number
  claimed_quota: number
  claimed_today: number
  daily_remaining: number | null
}

export type EmptyResponseCompensationAdminSummary = {
  total_count: number
  pending_count: number
  claimed_count: number
  blocked_count: number
  expired_count: number
  pending_quota: number
  claimed_quota: number
}

export type PageData<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type EmptyResponseCompensationUserData = {
  enabled: boolean
  rules: EmptyResponseCompensationRules
  summary: EmptyResponseCompensationSummary
  records: PageData<EmptyResponseCompensationRecord>
}

export type EmptyResponseCompensationAdminData = {
  rules: EmptyResponseCompensationRules
  summary: EmptyResponseCompensationAdminSummary
  records: PageData<EmptyResponseCompensationRecord>
}

export type EmptyResponseCompensationClaimResult = {
  claimed_ids: number[]
  skipped: Record<string, string>
  credited_quota: number
  claimed_count: number
}
