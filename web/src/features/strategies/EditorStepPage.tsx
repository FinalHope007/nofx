import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { AlertTriangle, ArrowLeft, Loader2, Save } from 'lucide-react'
import { useStrategyDraft } from './draftStore'
import { strategyManagerApi } from './strategyApi'
import {
  buildStrategyConfig,
  TRADING_STYLE_PRESETS,
  type TradingStyle,
} from './strategyFactory'
import { defaultDataSources } from './dataSourceDefaults'
import { notify } from '../../lib/notify'
import { api } from '../../lib/api'
import { useAuth } from '../../contexts/AuthContext'

type Mode = 'create' | 'edit'

const TIMEFRAMES = ['1m', '5m', '15m', '1h', '4h', '1d']
const DURATIONS = ['15m', '30m', '1h', '4h', '8h', '12h', '24h']
const EMA_PERIODS = [9, 10, 20, 50, 200]
const RSI_PERIODS = [7, 14, 21]
const ATR_PERIODS = [7, 14, 21]
const BOLL_PERIODS = [10, 20, 50]
const RANKING_DURATIONS = ['1h', '4h', '24h']

function toggleNumberList(current: number[], n: number): number[] {
  return current.includes(n) ? current.filter((x) => x !== n) : [...current, n].sort((a, b) => a - b)
}

export function EditorStepPage() {
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const mode: Mode = params.id ? 'edit' : 'create'
  const strategyId = params.id
  const { scope } = useStrategyDraft()
  const { token } = useAuth()
  const [loading, setLoading] = useState(mode === 'edit')
  const [blocked, setBlocked] = useState<string[]>([])

  const [name, setName] = useState('')
  const [prompt, setPrompt] = useState('')
  const [interval, setInterval] = useState(15)
  const [btcEthLeverage, setBtcEthLeverage] = useState(5)
  const [altLeverage, setAltLeverage] = useState(5)
  const [btcEthRatio, setBtcEthRatio] = useState(5)
  const [altRatio, setAltRatio] = useState(5)
  const [isCross, setIsCross] = useState(true)
  const [timeframes, setTimeframes] = useState<string[]>(['15m'])
  const [excluded, setExcluded] = useState('')
  const [decisionEnabled, setDecisionEnabled] = useState(true)
  const [decisionCount, setDecisionCount] = useState(8)
  const [contextMode, setContextMode] = useState<'structured' | 'digest'>(
    'structured'
  )
  const [tradingStyle, setTradingStyle] = useState<TradingStyle>('default')
  const [maxPositions, setMaxPositions] = useState(2)
  const [minPositionSize, setMinPositionSize] = useState(12)
  const [minRiskRewardRatio, setMinRiskRewardRatio] = useState(3.0)
  const [maxMarginUsage, setMaxMarginUsage] = useState(1.0)
  const [minConfidence, setMinConfidence] = useState(78)
  const [enableOILiquidityFilter, setEnableOILiquidityFilter] = useState(true)
  const [oiLiquidityFilterMinUSDT, setOILiquidityFilterMinUSDT] = useState(
    15000000
  )
  const [maxOpensPerHour, setMaxOpensPerHour] = useState(3)
  const [maxOpensPerCycle, setMaxOpensPerCycle] = useState(2)
  const [minHoldDurationMin, setMinHoldDurationMin] = useState(90)
  const [noiseCloseHoldDurationMin, setNoiseCloseHoldDurationMin] = useState(
    180
  )
  const [reentryCooldownMin, setReentryCooldownMin] = useState(240)
  const [earlyCloseStopLossBypassPct, setEarlyCloseStopLossBypassPct] =
    useState(-3.0)
  const [earlyCloseTakeProfitBypassPct, setEarlyCloseTakeProfitBypassPct] =
    useState(8.0)
  const [noiseCloseLossFloorPct, setNoiseCloseLossFloorPct] = useState(-2.0)
  const [noiseCloseProfitCeilingPct, setNoiseCloseProfitCeilingPct] =
    useState(3.0)
  const [saving, setSaving] = useState(false)

  const initialSources = defaultDataSources(scope)
  const [enableAI500Data, setEnableAI500Data] = useState(
    initialSources.enableAI500Data
  )
  const [enableOIData, setEnableOIData] = useState(initialSources.enableOIData)
  const [enableNetflowData, setEnableNetflowData] = useState(
    initialSources.enableNetflowData
  )
  const [enablePriceData, setEnablePriceData] = useState(
    initialSources.enablePriceData
  )
  const [enableEma, setEnableEma] = useState(false)
  const [enableMacd, setEnableMacd] = useState(false)
  const [enableRsi, setEnableRsi] = useState(false)
  const [enableOi, setEnableOi] = useState(false)
  const [enableFundingRate, setEnableFundingRate] = useState(false)
  const [enableAtr, setEnableAtr] = useState(false)
  const [enableBoll, setEnableBoll] = useState(false)
  const [enableVolume, setEnableVolume] = useState(false)
  const [emaPeriods, setEmaPeriods] = useState<number[]>([20, 50])
  const [rsiPeriods, setRsiPeriods] = useState<number[]>([7, 14])
  const [atrPeriods, setAtrPeriods] = useState<number[]>([14])
  const [bollPeriods, setBollPeriods] = useState<number[]>([20])
  const [enableBinanceTechnicalData, setEnableBinanceTechnicalData] = useState(false)
  const [enableBinanceSentimentData, setEnableBinanceSentimentData] = useState(false)
  const [binanceTechnicalIntervals, setBinanceTechnicalIntervals] = useState<('1h' | '24h')[]>(['1h'])
  const [primaryCount, setPrimaryCount] = useState(30)
  const [longerTimeframe, setLongerTimeframe] = useState('')
  const [longerCount, setLongerCount] = useState(0)
  const [enableOIRanking, setEnableOIRanking] = useState(false)
  const [oiRankingDuration, setOIRankingDuration] = useState('1h')
  const [oiRankingLimit, setOIRankingLimit] = useState(10)
  const [enableNetFlowRanking, setEnableNetFlowRanking] = useState(false)
  const [netFlowRankingDuration, setNetFlowRankingDuration] = useState('1h')
  const [netFlowRankingLimit, setNetFlowRankingLimit] = useState(10)
  const [enablePriceRanking, setEnablePriceRanking] = useState(false)
  const [priceRankingDuration, setPriceRankingDuration] = useState('1h')
  const [priceRankingLimit, setPriceRankingLimit] = useState(10)
  const [dataDurations, setDataDurations] = useState<string[]>(['1h', '24h'])

  useEffect(() => {
    if (mode !== 'edit' || !strategyId || !token) {
      setLoading(false)
      return
    }
    ;(async () => {
      try {
        const [strategy, running] = await Promise.all([
          api.getStrategy(strategyId),
          strategyManagerApi.getRunningTradersForStrategy(strategyId),
        ])
        if (running.length > 0) {
          setBlocked(running)
          setLoading(false)
          return
        }
        const ai = strategy.config.ai_config
        const ind = ai?.indicators
        const rc = ai?.risk_control
        const th = rc?.throttling
        setName(strategy.name)
        setPrompt(ai?.custom_prompt ?? '')
        setBtcEthLeverage(rc?.btc_eth_max_leverage ?? 5)
        setAltLeverage(rc?.altcoin_max_leverage ?? 5)
        setBtcEthRatio(rc?.btc_eth_max_position_value_ratio ?? 5)
        setAltRatio(rc?.altcoin_max_position_value_ratio ?? 5)
        setTradingStyle(strategy.config.trading_style ?? 'default')
        setMaxPositions(rc?.max_positions ?? 2)
        setMinPositionSize(rc?.min_position_size ?? 12)
        setMinRiskRewardRatio(rc?.min_risk_reward_ratio ?? 3.0)
        setMaxMarginUsage(rc?.max_margin_usage ?? 1.0)
        setMinConfidence(rc?.min_confidence ?? 78)
        setEnableOILiquidityFilter(rc?.enable_oi_liquidity_filter ?? true)
        setOILiquidityFilterMinUSDT(rc?.oi_liquidity_filter_min_usdt ?? 15000000)
        setMaxOpensPerHour(th?.max_opens_per_hour ?? 3)
        setMaxOpensPerCycle(th?.max_opens_per_cycle ?? 2)
        setMinHoldDurationMin(th?.min_hold_duration_min ?? 90)
        setNoiseCloseHoldDurationMin(th?.noise_close_hold_duration_min ?? 180)
        setReentryCooldownMin(th?.reentry_cooldown_min ?? 240)
        setEarlyCloseStopLossBypassPct(
          th?.early_close_stop_loss_bypass_pct ?? -3.0
        )
        setEarlyCloseTakeProfitBypassPct(
          th?.early_close_take_profit_bypass_pct ?? 8.0
        )
        setNoiseCloseLossFloorPct(th?.noise_close_loss_floor_pct ?? -2.0)
        setNoiseCloseProfitCeilingPct(th?.noise_close_profit_ceiling_pct ?? 3.0)
        setTimeframes(ind?.klines.selected_timeframes ?? ['15m'])
        setExcluded((ai?.coin_source.excluded_coins ?? []).join(', '))

        const hasDataSourceFlags =
          ind?.enable_ai500_data !== undefined ||
          ind?.enable_oi_data !== undefined ||
          ind?.enable_netflow_data !== undefined ||
          ind?.enable_price_data !== undefined
        if (hasDataSourceFlags) {
          setEnableAI500Data(ind?.enable_ai500_data ?? false)
          setEnableOIData(ind?.enable_oi_data ?? false)
          setEnableNetflowData(ind?.enable_netflow_data ?? false)
          setEnablePriceData(ind?.enable_price_data ?? false)
        } else {
          const defaults = defaultDataSources(scope)
          setEnableAI500Data(defaults.enableAI500Data)
          setEnableOIData(defaults.enableOIData)
          setEnableNetflowData(defaults.enableNetflowData)
          setEnablePriceData(defaults.enablePriceData)
        }
        setEnableEma(ind?.enable_ema ?? false)
        setEnableMacd(ind?.enable_macd ?? false)
        setEnableRsi(ind?.enable_rsi ?? false)
        setEnableOi(ind?.enable_oi ?? false)
        setEnableFundingRate(ind?.enable_funding_rate ?? false)
        setEnableAtr(ind?.enable_atr ?? false)
        setEnableBoll(ind?.enable_boll ?? false)
        setEnableVolume(ind?.enable_volume ?? false)
        setEmaPeriods(ind?.ema_periods?.length ? ind.ema_periods : [20, 50])
        setRsiPeriods(ind?.rsi_periods?.length ? ind.rsi_periods : [7, 14])
        setAtrPeriods(ind?.atr_periods?.length ? ind.atr_periods : [14])
        setBollPeriods(ind?.boll_periods?.length ? ind.boll_periods : [20])
        setEnableBinanceTechnicalData(ind?.enable_binance_technical_data ?? false)
        setEnableBinanceSentimentData(ind?.enable_binance_sentiment_data ?? false)
        setBinanceTechnicalIntervals(ind?.binance_technical_intervals?.length ? ind.binance_technical_intervals : ['1h'])
        setPrimaryCount(ind?.klines.primary_count ?? 30)
        setLongerTimeframe(ind?.klines.longer_timeframe ?? '')
        setLongerCount(ind?.klines.longer_count ?? 0)
        setEnableOIRanking(ind?.enable_oi_ranking ?? false)
        setOIRankingDuration(ind?.oi_ranking_duration ?? '1h')
        setOIRankingLimit(ind?.oi_ranking_limit ?? 10)
        setEnableNetFlowRanking(ind?.enable_netflow_ranking ?? false)
        setNetFlowRankingDuration(ind?.netflow_ranking_duration ?? '1h')
        setNetFlowRankingLimit(ind?.netflow_ranking_limit ?? 10)
        setEnablePriceRanking(ind?.enable_price_ranking ?? false)
        setPriceRankingDuration(ind?.price_ranking_duration ?? '1h')
        setPriceRankingLimit(ind?.price_ranking_limit ?? 10)
        setDataDurations(
          ind?.data_durations && ind.data_durations.length
            ? ind.data_durations
            : ['1h', '24h']
        )

        // Restore decision-context settings from the saved config. These are
        // NOT restored previously, so opening the editor reset them to their
        // useState defaults (enabled=true, count=8, mode=structured), and a
        // subsequent save would overwrite the user's recent_count.
        const dc = ai?.decision_context
        setDecisionEnabled(dc?.enabled ?? true)
        setDecisionCount(dc?.recent_count ?? 8)
        setContextMode(dc?.mode ?? 'structured')
      } catch (err) {
        notify.error(
          err instanceof Error ? err.message : 'Failed to load strategy'
        )
      } finally {
        setLoading(false)
      }
    })()
  }, [mode, strategyId, token, scope])

  const backPath =
    mode === 'create'
      ? '/strategy/create/scope'
      : `/strategy/${strategyId}/edit/scope`

  const toggleTimeframe = (tf: string) => {
    setTimeframes((prev) =>
      prev.includes(tf) ? prev.filter((t) => t !== tf) : [...prev, tf]
    )
  }

  const toggleDuration = (d: string) => {
    setDataDurations((prev) =>
      prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d]
    )
  }

  const applyStyle = (style: TradingStyle) => {
    setTradingStyle(style)
    const patch = TRADING_STYLE_PRESETS[style]
    if (patch.maxPositions !== undefined) setMaxPositions(patch.maxPositions)
    if (patch.minPositionSize !== undefined)
      setMinPositionSize(patch.minPositionSize)
    if (patch.minRiskRewardRatio !== undefined)
      setMinRiskRewardRatio(patch.minRiskRewardRatio)
    if (patch.maxOpensPerHour !== undefined)
      setMaxOpensPerHour(patch.maxOpensPerHour)
    if (patch.maxOpensPerCycle !== undefined)
      setMaxOpensPerCycle(patch.maxOpensPerCycle)
    if (patch.minHoldDurationMin !== undefined)
      setMinHoldDurationMin(patch.minHoldDurationMin)
    if (patch.noiseCloseHoldDurationMin !== undefined)
      setNoiseCloseHoldDurationMin(patch.noiseCloseHoldDurationMin)
    if (patch.reentryCooldownMin !== undefined)
      setReentryCooldownMin(patch.reentryCooldownMin)
  }

  const handleSave = async () => {
    if (saving) return
    if (!name.trim()) {
      notify.error('Strategy name is required')
      return
    }
    if (mode === 'edit' && scope === null) {
      notify.error('No trading scope selected. Finish step 1 before saving.')
      return
    }
    setSaving(true)
    try {
      const config = buildStrategyConfig({
        name,
        custom_prompt: prompt,
        scan_interval_minutes: Math.max(3, interval),
        btcEthMaxLeverage: btcEthLeverage,
        altcoinMaxLeverage: altLeverage,
        btcEthPositionRatio: btcEthRatio,
        altcoinPositionRatio: altRatio,
        isCrossMargin: isCross,
        selectedTimeframes: timeframes.length ? timeframes : ['15m'],
        excludedCoins: excluded
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
        decisionContext: {
          enabled: decisionEnabled,
          recent_count: decisionCount,
          mode: contextMode,
        },
        enableAI500Data,
        enableOIData,
        enableNetflowData,
        enablePriceData,
        dataDurations,
        enableEma,
        enableMacd,
        enableRsi,
        enableOi,
        enableFundingRate,
        enableAtr,
        enableBoll,
        enableVolume,
        emaPeriods,
        rsiPeriods,
        atrPeriods,
        bollPeriods,
        enableBinanceTechnicalData,
        enableBinanceSentimentData,
        binanceTechnicalIntervals,
        primaryCount,
        longerTimeframe,
        longerCount,
        enableOIRanking,
        oiRankingDuration,
        oiRankingLimit,
        enableNetFlowRanking,
        netFlowRankingDuration,
        netFlowRankingLimit,
        enablePriceRanking,
        priceRankingDuration,
        priceRankingLimit,
        tradingStyle,
        maxPositions,
        minPositionSize,
        minRiskRewardRatio,
        maxMarginUsage,
        minConfidence,
        enableOILiquidityFilter,
        oiLiquidityFilterMinUSDT,
        maxOpensPerHour,
        maxOpensPerCycle,
        minHoldDurationMin,
        noiseCloseHoldDurationMin,
        reentryCooldownMin,
        earlyCloseStopLossBypassPct,
        earlyCloseTakeProfitBypassPct,
        noiseCloseLossFloorPct,
        noiseCloseProfitCeilingPct,
        scopeUnit: scope,
      })

      if (mode === 'create') {
        await strategyManagerApi.createStrategy({
          name: name.trim(),
          description: `Strategy using ${scope ? 1 : 0} scope(s)`,
          config,
        })
        notify.success('Strategy created')
      } else if (strategyId) {
        await strategyManagerApi.updateStrategy(strategyId, {
          name: name.trim(),
          config,
        })
        notify.success('Strategy saved')
      }
      navigate('/strategy')
    } catch (err) {
      notify.error(
        err instanceof Error ? err.message : 'Failed to save strategy'
      )
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-7 w-7 animate-spin text-nofx-gold" />
      </div>
    )
  }

  if (blocked.length > 0) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <div className="rounded-lg border border-nofx-danger/30 bg-nofx-danger/10 p-6">
          <h1 className="text-lg font-semibold text-nofx-text">
            This strategy is in use
          </h1>
          <p className="mt-2 text-sm text-nofx-text-muted">
            Editing a strategy while a trader is live on it can cause
            discrepancies between open positions and your parameters (for
            example, leverage changes). Stop the following trader
            {blocked.length > 1 ? 's' : ''} before editing this strategy:
          </p>
          <ul className="mt-3 list-inside list-disc text-sm text-nofx-danger">
            {blocked.map((name) => (
              <li key={name}>{name}</li>
            ))}
          </ul>
          <button
            type="button"
            onClick={() => navigate('/traders')}
            className="mt-5 rounded-lg bg-nofx-gold px-4 py-2 text-sm font-semibold text-nofx-bg"
          >
            Go to Traders to stop it
          </button>
          <button
            type="button"
            onClick={() => navigate('/strategy')}
            className="ml-3 mt-5 rounded-lg border border-[rgba(26,24,19,0.14)] px-4 py-2 text-sm text-nofx-text-muted hover:text-nofx-text"
          >
            Back to strategies
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl p-6">
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => navigate(backPath)}
            className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-2 text-nofx-text-muted hover:text-nofx-text"
            aria-label="Back to scope"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <h1 className="text-xl font-semibold text-nofx-text">
              {mode === 'create' ? 'New Strategy' : 'Edit Strategy'}
            </h1>
            <p className="mt-1 text-sm text-nofx-text-muted">
              Step 2 of 2 · Enter Trading Strategy
            </p>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        <label className="block">
          <span className="text-sm font-medium text-nofx-text">
            Strategy Name
          </span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper px-3 py-2 text-sm text-nofx-text"
            placeholder="e.g. Hyper Top Gainers"
          />
        </label>

        <label className="block">
          <span className="text-sm font-medium text-nofx-text">
            Trading Strategy Prompt
          </span>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={4}
            className="mt-1 w-full resize-none rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper px-3 py-2 text-sm text-nofx-text"
            placeholder="Instructions for the AI trading loop..."
          />
        </label>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Trading Style
          </legend>
          <div className="flex flex-wrap gap-2">
            {(['scalp', 'intraday', 'swing', 'default'] as TradingStyle[]).map(
              (s) => (
                <ToggleChip
                  key={s}
                  label={s[0].toUpperCase() + s.slice(1)}
                  active={tradingStyle === s}
                  onClick={() => applyStyle(s)}
                />
              )
            )}
          </div>
        </fieldset>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Basic Rules
          </legend>
          <div className="grid gap-4 sm:grid-cols-2">
            <NumberField
              label="AI decision interval (min)"
              value={interval}
              onChange={setInterval}
              min={3}
            />
            <NumberField
              label="Max position leverage — BTC/ETH"
              value={btcEthLeverage}
              onChange={setBtcEthLeverage}
              min={1}
              max={20}
            />
            <NumberField
              label="Max position leverage — Altcoin"
              value={altLeverage}
              onChange={setAltLeverage}
              min={1}
              max={20}
            />
            <NumberField
              label="Max account leverage (notional × equity) — BTC/ETH"
              value={btcEthRatio}
              onChange={setBtcEthRatio}
              min={0.5}
              max={10}
              step={0.5}
            />
            <NumberField
              label="Max account leverage (notional × equity) — Altcoin"
              value={altRatio}
              onChange={setAltRatio}
              min={0.5}
              max={10}
              step={0.5}
            />
            <NumberField
              label="Max concurrent positions"
              value={maxPositions}
              onChange={setMaxPositions}
              min={1}
              max={8}
            />
            <NumberField
              label="Min position size (USDT)"
              value={minPositionSize}
              onChange={setMinPositionSize}
              min={10}
              max={1000}
            />
          </div>
        </fieldset>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Basic indicators
          </legend>
          <div className="mb-4 flex flex-wrap gap-2">
            <ToggleChip label="EMA" active={enableEma} onClick={() => setEnableEma(!enableEma)} />
            <ToggleChip label="MACD" active={enableMacd} onClick={() => setEnableMacd(!enableMacd)} />
            <ToggleChip label="RSI" active={enableRsi} onClick={() => setEnableRsi(!enableRsi)} />
            <ToggleChip label="ATR" active={enableAtr} onClick={() => setEnableAtr(!enableAtr)} />
            <ToggleChip label="BOLL" active={enableBoll} onClick={() => setEnableBoll(!enableBoll)} />
            <ToggleChip label="Volume" active={enableVolume} onClick={() => setEnableVolume(!enableVolume)} />
          </div>

          {enableEma && (
            <div className="mb-4">
              <span className="text-sm text-nofx-text-muted">EMA periods</span>
              <div className="mt-1 flex flex-wrap gap-2">
                {EMA_PERIODS.map((p) => (
                  <ToggleChip key={p} label={String(p)} active={emaPeriods.includes(p)} onClick={() => setEmaPeriods(toggleNumberList(emaPeriods, p))} />
                ))}
              </div>
            </div>
          )}

          {enableRsi && (
            <div className="mb-4">
              <span className="text-sm text-nofx-text-muted">RSI periods</span>
              <div className="mt-1 flex flex-wrap gap-2">
                {RSI_PERIODS.map((p) => (
                  <ToggleChip key={p} label={String(p)} active={rsiPeriods.includes(p)} onClick={() => setRsiPeriods(toggleNumberList(rsiPeriods, p))} />
                ))}
              </div>
            </div>
          )}

          {enableAtr && (
            <div className="mb-4">
              <span className="text-sm text-nofx-text-muted">ATR periods</span>
              <div className="mt-1 flex flex-wrap gap-2">
                {ATR_PERIODS.map((p) => (
                  <ToggleChip key={p} label={String(p)} active={atrPeriods.includes(p)} onClick={() => setAtrPeriods(toggleNumberList(atrPeriods, p))} />
                ))}
              </div>
            </div>
          )}

          {enableBoll && (
            <div className="mb-4">
              <span className="text-sm text-nofx-text-muted">BOLL periods</span>
              <div className="mt-1 flex flex-wrap gap-2">
                {BOLL_PERIODS.map((p) => (
                  <ToggleChip key={p} label={String(p)} active={bollPeriods.includes(p)} onClick={() => setBollPeriods(toggleNumberList(bollPeriods, p))} />
                ))}
              </div>
            </div>
          )}

          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Candles</span>
            <div className="mt-1 flex flex-wrap items-center gap-3">
              <div className="flex items-center gap-1">
                <span className="text-xs text-nofx-text-muted">Primary count</span>
                <input
                  type="number"
                  min={10}
                  max={200}
                  value={primaryCount}
                  onChange={(e) => setPrimaryCount(Number(e.target.value))}
                  className="w-20 rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text"
                />
              </div>
              <div className="flex flex-wrap gap-2">
                {TIMEFRAMES.map((tf) => (
                  <ToggleChip key={tf} label={tf} active={timeframes.includes(tf)} onClick={() => toggleTimeframe(tf)} />
                ))}
              </div>
            </div>
          </div>

          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Multi-timeframe</span>
            <div className="mt-1 flex flex-wrap items-center gap-3">
              <select
                value={longerTimeframe}
                onChange={(e) => setLongerTimeframe(e.target.value)}
                className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text"
              >
                <option value="">Disabled</option>
                {TIMEFRAMES.map((tf) => (
                  <option key={tf} value={tf}>{tf}</option>
                ))}
              </select>
              {longerTimeframe && (
                <div className="flex items-center gap-1">
                  <span className="text-xs text-nofx-text-muted">Count</span>
                  <input
                    type="number"
                    min={0}
                    max={200}
                    value={longerCount}
                    onChange={(e) => setLongerCount(Number(e.target.value))}
                    className="w-20 rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text"
                  />
                </div>
              )}
            </div>
          </div>
        </fieldset>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Data sources
          </legend>
          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Per-coin data</span>
            <div className="mt-1 flex flex-wrap gap-2">
              <ToggleChip label="AI500" active={enableAI500Data} onClick={() => setEnableAI500Data(!enableAI500Data)} />
              <ToggleChip label="OI" active={enableOIData} onClick={() => setEnableOIData(!enableOIData)} />
              <ToggleChip label="Netflow" active={enableNetflowData} onClick={() => setEnableNetflowData(!enableNetflowData)} />
              <ToggleChip label="Price" active={enablePriceData} onClick={() => setEnablePriceData(!enablePriceData)} />
            </div>
          </div>

          {(enableOIData || enableNetflowData || enablePriceData) && (
            <div className="mb-4">
              <span className="text-sm text-nofx-text-muted">Data durations</span>
              <div className="mt-1 flex flex-wrap gap-2">
                {DURATIONS.map((d) => (
                  <ToggleChip key={d} label={d} active={dataDurations.includes(d)} onClick={() => toggleDuration(d)} />
                ))}
              </div>
            </div>
          )}

          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Binance data</span>
            <div className="mt-1 flex flex-wrap gap-2">
              <ToggleChip label="Binance Technical" active={enableBinanceTechnicalData} onClick={() => setEnableBinanceTechnicalData(!enableBinanceTechnicalData)} />
              <ToggleChip label="Binance Sentiment" active={enableBinanceSentimentData} onClick={() => setEnableBinanceSentimentData(!enableBinanceSentimentData)} />
            </div>
            <div className="mt-2 flex items-start gap-2 rounded-md border border-nofx-danger/25 bg-nofx-danger/10 p-2">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-nofx-danger" />
              <p className="text-[11px] leading-snug text-nofx-danger">
                Binance discontinued this data feed. These toggles are kept for reference and will produce no data.
              </p>
            </div>
          </div>

          {enableBinanceTechnicalData && (
            <div className="mb-4">
              <span className="text-sm text-nofx-text-muted">Binance technical intervals</span>
              <div className="mt-1 flex flex-wrap gap-2">
                {(['1h', '24h'] as const).map((iv) => (
                  <ToggleChip
                    key={iv}
                    label={iv}
                    active={binanceTechnicalIntervals.includes(iv)}
                    onClick={() =>
                      setBinanceTechnicalIntervals((prev) =>
                        prev.includes(iv) ? prev.filter((x) => x !== iv) : [...prev, iv]
                      )
                    }
                  />
                ))}
              </div>
            </div>
          )}

          <hr className="my-4 border-[rgba(26,24,19,0.14)]" />
          <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-nofx-text-muted">Rankings</h4>

          <div className="mb-3">
            <ToggleChip label="OI Ranking" active={enableOIRanking} onClick={() => setEnableOIRanking(!enableOIRanking)} />
            {enableOIRanking && (
              <div className="mt-2 flex items-center gap-3">
                <select value={oiRankingDuration} onChange={(e) => setOIRankingDuration(e.target.value)} className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text">
                  {RANKING_DURATIONS.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
                <div className="flex items-center gap-1">
                  <span className="text-xs text-nofx-text-muted">Limit</span>
                  <input type="number" min={1} max={50} value={oiRankingLimit} onChange={(e) => setOIRankingLimit(Number(e.target.value))} className="w-20 rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text" />
                </div>
              </div>
            )}
          </div>

          <div className="mb-3">
            <ToggleChip label="Netflow Ranking" active={enableNetFlowRanking} onClick={() => setEnableNetFlowRanking(!enableNetFlowRanking)} />
            {enableNetFlowRanking && (
              <div className="mt-2 flex items-center gap-3">
                <select value={netFlowRankingDuration} onChange={(e) => setNetFlowRankingDuration(e.target.value)} className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text">
                  {RANKING_DURATIONS.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
                <div className="flex items-center gap-1">
                  <span className="text-xs text-nofx-text-muted">Limit</span>
                  <input type="number" min={1} max={50} value={netFlowRankingLimit} onChange={(e) => setNetFlowRankingLimit(Number(e.target.value))} className="w-20 rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text" />
                </div>
              </div>
            )}
          </div>

          <div className="mb-3">
            <ToggleChip label="Price Ranking" active={enablePriceRanking} onClick={() => setEnablePriceRanking(!enablePriceRanking)} />
            {enablePriceRanking && (
              <div className="mt-2 flex items-center gap-3">
                <select value={priceRankingDuration} onChange={(e) => setPriceRankingDuration(e.target.value)} className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text">
                  {RANKING_DURATIONS.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
                <div className="flex items-center gap-1">
                  <span className="text-xs text-nofx-text-muted">Limit</span>
                  <input type="number" min={1} max={50} value={priceRankingLimit} onChange={(e) => setPriceRankingLimit(Number(e.target.value))} className="w-20 rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-2 py-1 text-sm text-nofx-text" />
                </div>
              </div>
            )}
          </div>
        </fieldset>

        <fieldset className="rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg-deeper p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-text">
            Advanced Settings
          </legend>
          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">Position mode</span>
            <div className="mt-1 flex gap-2">
              <ToggleChip
                label="Cross margin"
                active={isCross}
                onClick={() => setIsCross(true)}
              />
              <ToggleChip
                label="Isolated"
                active={!isCross}
                onClick={() => setIsCross(false)}
              />
            </div>
          </div>

          <label className="mb-4 block">
            <span className="text-sm text-nofx-text-muted">
              Excluded coins (comma separated)
            </span>
            <input
              type="text"
              value={excluded}
              onChange={(e) => setExcluded(e.target.value)}
              className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-3 py-2 text-sm text-nofx-text"
              placeholder="SAMECOIN, JUNKCOIN"
            />
          </label>

          <div className="mb-4 grid gap-4 sm:grid-cols-2">
            <NumberField
              label="Min risk/reward ratio"
              value={minRiskRewardRatio}
              onChange={setMinRiskRewardRatio}
              min={1.0}
              max={10.0}
              step={0.5}
            />
            <NumberField
              label="Max margin usage (%) — AI guidance"
              value={maxMarginUsage}
              onChange={setMaxMarginUsage}
              min={0.1}
              max={1.0}
              step={0.1}
            />
            <NumberField
              label="Min AI confidence"
              value={minConfidence}
              onChange={setMinConfidence}
              min={50}
              max={100}
            />
          </div>

          <div className="mb-4">
            <span className="text-sm text-nofx-text-muted">
              OI-liquidity filter
            </span>
            <Toggle
              checked={enableOILiquidityFilter}
              onChange={() => setEnableOILiquidityFilter(!enableOILiquidityFilter)}
              label={enableOILiquidityFilter ? 'On' : 'Off'}
            />
            <input
              type="number"
              value={oiLiquidityFilterMinUSDT}
              onChange={(e) =>
                setOILiquidityFilterMinUSDT(Number(e.target.value))
              }
              disabled={!enableOILiquidityFilter}
              className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-3 py-2 text-sm text-nofx-text disabled:opacity-40"
              min={0}
            />
          </div>

          <div className="rounded-md border border-nofx-danger/25 bg-nofx-danger/10 p-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-nofx-text">
                Recent decisions context
              </span>
              <Toggle
                checked={decisionEnabled}
                onChange={() => setDecisionEnabled(!decisionEnabled)}
                label={decisionEnabled ? 'On' : 'Off'}
              />
            </div>
            <div className="mt-3 grid gap-3 text-nofx-text-muted sm:grid-cols-2">
              <NumberField
                label="Decisions in context"
                value={decisionCount}
                onChange={setDecisionCount}
                min={1}
                max={50}
              />
              <div>
                <span className="text-sm">Context mode</span>
                <div className="mt-1 flex gap-2">
                  <ToggleChip
                    label="Structured"
                    active={contextMode === 'structured'}
                    onClick={() => setContextMode('structured')}
                  />
                  <ToggleChip
                    label="Digest"
                    active={contextMode === 'digest'}
                    onClick={() => setContextMode('digest')}
                  />
                </div>
              </div>
            </div>
          </div>
        </fieldset>

        <fieldset className="rounded-lg border border-nofx-danger/30 bg-nofx-danger/5 p-4">
          <legend className="px-2 text-sm font-semibold text-nofx-danger">
            Throttling Settings (Risky)
          </legend>
          <div className="grid gap-4 sm:grid-cols-2">
            <NumberField
              label="Max opens per hour"
              value={maxOpensPerHour}
              onChange={setMaxOpensPerHour}
              min={1}
              max={100}
            />
            <NumberField
              label="Max opens per cycle"
              value={maxOpensPerCycle}
              onChange={setMaxOpensPerCycle}
              min={1}
              max={100}
            />
            <NumberField
              label="Min hold before close (min)"
              value={minHoldDurationMin}
              onChange={setMinHoldDurationMin}
              min={1}
              max={10080}
            />
            <NumberField
              label="Noise-band close window (min)"
              value={noiseCloseHoldDurationMin}
              onChange={setNoiseCloseHoldDurationMin}
              min={1}
              max={10080}
            />
            <NumberField
              label="Re-entry cooldown (min)"
              value={reentryCooldownMin}
              onChange={setReentryCooldownMin}
              min={1}
              max={10080}
            />
            <NumberField
              label="Early-close stop-loss bypass (%)"
              value={earlyCloseStopLossBypassPct}
              onChange={setEarlyCloseStopLossBypassPct}
              min={-100}
              max={0}
              step={0.5}
            />
            <NumberField
              label="Early-close take-profit bypass (%)"
              value={earlyCloseTakeProfitBypassPct}
              onChange={setEarlyCloseTakeProfitBypassPct}
              min={0}
              max={100}
              step={0.5}
            />
            <NumberField
              label="Noise-band loss floor (%)"
              value={noiseCloseLossFloorPct}
              onChange={setNoiseCloseLossFloorPct}
              min={-100}
              max={0}
              step={0.5}
            />
            <NumberField
              label="Noise-band profit ceiling (%)"
              value={noiseCloseProfitCeilingPct}
              onChange={setNoiseCloseProfitCeilingPct}
              min={0}
              max={100}
              step={0.5}
            />
          </div>
        </fieldset>
      </div>

      <div className="mt-8 flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="inline-flex items-center gap-2 rounded-lg bg-nofx-gold px-5 py-2 text-sm font-semibold text-nofx-bg disabled:cursor-not-allowed disabled:opacity-40"
        >
          {saving ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Save className="h-4 w-4" />
          )}
          Save Strategy
        </button>
      </div>
    </div>
  )
}

function NumberField({
  label,
  value,
  onChange,
  min,
  max,
  step = 1,
}: {
  label: string
  value: number
  onChange: (n: number) => void
  min: number
  max?: number
  step?: number
}) {
  return (
    <label className="block">
      <span className="text-sm text-nofx-text-muted">{label}</span>
      <input
        type="number"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="mt-1 w-full rounded-lg border border-[rgba(26,24,19,0.14)] bg-nofx-bg px-3 py-2 text-sm text-nofx-text"
      />
    </label>
  )
}

function ToggleChip({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-lg border px-3 py-2 text-sm ${
        active
          ? 'border-nofx-gold bg-nofx-gold/10 text-nofx-gold'
          : 'border-[rgba(26,24,19,0.14)] text-nofx-text-muted'
      }`}
    >
      {label}
    </button>
  )
}

function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: () => void
  label: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={onChange}
      className="inline-flex items-center gap-2"
    >
      <span
        data-testid="decision-toggle-track"
        className={`relative inline-flex h-6 w-11 items-center rounded-full border transition-colors ${
          checked
            ? 'border-nofx-gold bg-nofx-gold'
            : 'border-[rgba(26,24,19,0.25)] bg-nofx-bg-deeper'
        }`}
      >
        <span
          className={`inline-block h-4 w-4 transform rounded-full bg-nofx-bg-lighter shadow transition-transform ${
            checked ? 'translate-x-6' : 'translate-x-0.5'
          }`}
        />
      </span>
      <span
        className={`text-xs font-medium ${
          checked ? 'text-nofx-text' : 'text-nofx-text-muted'
        }`}
      >
        {label}
      </span>
    </button>
  )
}
