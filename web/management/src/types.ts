export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  message: string
  type: ToastType
}

export interface ConfirmDialog {
  title: string
  body: string
  confirmText: string
  callback?: () => void | Promise<void>
  requiresInput?: boolean
  inputVal: string
  loading: boolean
}

export interface ApiKey {
  id: string
  name?: string
  is_active?: boolean
  scopes?: string[]
  rate_limit_override?: number | null
  created_at?: string
  expires_at?: string | null
  raw_key?: string
}

export interface Endpoint {
  id: string
  state?: string
  tags?: string[]
  version?: string
  active_tasks?: number
  last_seen?: string
}

export interface EndpointPool {
  tier: number
  max_retries?: number
  endpoint_ids?: string[]
}

export interface FingerprintVariant {
  preset_id: string
  weight: number
}

export interface FingerprintABTest {
  strategy?: string
  variants?: FingerprintVariant[]
}

export interface RequestFilters {
  enable_adblock?: boolean
  adblock_lists?: string[]
  block_content_types?: string[]
  block_url_patterns?: string[]
  block_domains?: string[]
}

export interface RoutingRule {
  id: string
  name: string
  priority: number
  version?: number
  is_active?: boolean
  required_tags?: string[]
  excluded_tags?: string[]
  allowed_endpoint_types?: string[]
  required_endpoint_caps?: string[]
  endpoint_pools?: EndpointPool[]
  fingerprint_preset?: string | null
  fingerprint_ab_test?: FingerprintABTest | null
  rate_limit_per_minute?: number | null
  rate_limit_per_second?: number | null
  hard_timeout?: string | null
  quota_key?: string | null
  allow_insecure_tls?: boolean
  pinned_cert_hash?: string | null
  request_filters?: RequestFilters
  created_at?: string
  updated_at?: string
}

export interface FingerprintPreset {
  id: string
  name?: string
  config?: Record<string, unknown> & {
    user_agent?: string
    UserAgent?: string
  }
  updated_at?: string
}

export interface UsageBreakdown {
  tier?: string
  requests?: number
}

export interface UsageDay {
  date?: string
  requests?: number
  bytes?: number
  cost_units?: number
  breakdown?: UsageBreakdown[]
}

export interface UsageSummary {
  total_requests?: number
  total_bytes?: number
  total_cost_units?: number
  daily?: UsageDay[]
}

export interface BillingEstimate {
  total_cost_units?: number
  estimated_usd?: number
  currency?: string
}

export interface CacheStats {
  status?: string
  redis_connected?: boolean
  info?: string
}

export interface ClearCacheResult {
  pattern?: string
  deleted: number
}

export interface OverviewData {
  endpoints?: Endpoint[]
  rules?: RoutingRule[]
  apiKeys?: ApiKey[]
  usage?: UsageSummary
  billing?: BillingEstimate
  cacheStats?: CacheStats
  fingerprints?: FingerprintPreset[]
}

export interface AppState {
  baseUrl: string
  token: string
  remember: boolean
  currentPage: string
  isLoading: boolean
  error: string | null
  toast: Toast | null
  confirmDialog: ConfirmDialog | null

  overviewData: OverviewData | null
  overviewLoading?: boolean
  overviewErrors?: Record<string, string | null> | null

  apiKeysData: ApiKey[] | { keys?: ApiKey[]; data?: ApiKey[] } | null
  apiKeysLoading?: boolean
  apiKeysError?: string | null
  apiKeysFilterStatus?: string
  apiKeysFilterScope?: string
  apiKeysFilterSearch?: string
  apiKeysBulkSelected?: string[]
  apiKeysScopeSuggestions?: string[]
  apiKeysShowCreateModal?: boolean
  apiKeysNewChips?: string[]
  apiKeysRawKey?: string | null
  apiKeysSuggestions?: ApiKey[]

  rulesData: RoutingRule[] | null
  rulesLoading?: boolean
  rulesError?: string | null
  rulesFilterStatus?: string
  rulesFilterTag?: string
  rulesFilterPreset?: string
  rulesFilterSearch?: string
  selectedRuleDetail?: RoutingRule | null
  selectedRuleLoading?: boolean
  selectedRuleTab?: string
  editingRule?: Partial<RoutingRule> | null
  editingRuleId?: string | null
  ruleJsonError?: string | null

  endpointsData: Endpoint[] | null
  endpointsLoading?: boolean
  endpointsFilterStatus?: string
  endpointsFilterTag?: string
  endpointsFilterVersion?: string
  endpointsFilterAge?: string
  endpointsFilterSearch?: string

  fingerprintsData: FingerprintPreset[] | null
  fingerprintsLoading?: boolean
  fingerprintsError?: string | null
  fingerprintsShowModal?: boolean
  fingerprintsModalIsEdit?: boolean
  fingerprintsModalIsDuplicate?: boolean
  fingerprintsModalRule?: Partial<FingerprintPreset> | null

  usageData: UsageSummary | null
  usageLoading?: boolean
  usageError?: string | null
  usageDatePreset?: string
  usageCustomStart?: string
  usageCustomEnd?: string
  usageApiKeyFilter?: string
  usageDateError?: string
  usageViewMode?: string
  billingData?: BillingEstimate | null

  cacheData: CacheStats | null
  cacheLoading?: boolean
  cacheError?: string | null
  cacheUnavailable?: boolean
  cacheClearPattern?: string
  cacheClearConfirmText?: string
  cacheClearLoading?: boolean
  cacheClearResult?: ClearCacheResult | null
  cacheInfoSearch?: string

  systemData: unknown
  systemHealth?: unknown
  systemHealthError?: string | null
  systemHealthLoading?: boolean
  systemHealthResponseTime?: number | null
  systemCapabilities?: Record<string, boolean> | null
  systemCapabilitiesLoading?: boolean
  systemLastChecked?: number | null

  loginError?: string | null
  lastRefreshed?: number
}

export interface Page {
  render(state: AppState): string
  refresh?(state: AppState): void | Promise<void>
  afterRender?(state: AppState): void | Promise<void>
}
