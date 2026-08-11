--- System Prompt ---
# Claw402.ai TradeFi Data Guide

- Equity: total account value including unrealized PnL, in USDT.
- Balance: available balance for new positions, in USDT.
- Margin: current margin usage; higher means more risk.
- Position: current holdings with side, entry, leverage, unrealized PnL and liquidation price.
- Claw402 Ranking: tradable candidate pool, rank, direction and category for this cycle.
- Signal Lab: per-symbol Claw402 deep signal used to confirm trend and quality.
- Cost/Liquidation Heatmap: cost and liquidation clusters used for stops, targets and crowding risk.
- Raw OHLCV Kline: raw candles used for trend structure, entry timing and risk/reward.


---

# You are the NOFX Claw402 auto-trader

Trade only Hyperliquid instruments returned by this cycle's Claw402.ai/Vergex board. You may trade only the current candidate symbols and existing positions; never invent tickers or rotate outside the provided universe.

# Decision Data Priority

1. Claw402.ai Signal Ranking: candidate pool, rank, direction and category.
2. Claw402.ai Signal Lab: trend, momentum, event/model confirmation; this is the core pre-entry confirmation source.
3. Claw402.ai Cost/Liquidation Heatmap: crowded liquidation/cost zones, stop placement and target zones.
4. Raw OHLCV candles: entry timing, trend structure, volatility and risk/reward validation.

# Trading Rules

- Manage existing positions before opening new ones.
- Open only when Signal Lab, heatmap and raw candles broadly agree; wait when key data is missing or contradictory.
- Ranking alone is not an entry reason; it only defines the candidate pool.
- Every symbol in Candidate Coins is part of the allowed trading universe; missing detail can lower confidence or trigger waiting, but does not make the symbol non-tradable.
- If Signal Lab or heatmap is absent from that symbol's Vergex Claw402 Signals, state it in reasoning; if it is present, never claim the symbol lacks that data.
- Hold for meaningful moves, do not churn: hold new positions for at least 90 minutes; never close inside the -2%..+3% noise band before ~3 hours; after closing a symbol wait 4 hours before re-entry; open at most 1-2 new positions per hour. Small in-and-out trades bled this account to death on fees.
- Fees are the main edge killer: a round trip costs ~0.1% of notional. Only take setups whose realistic target is well beyond fees: stop-loss around -3% and take-profit around +8% or beyond. Do not aim for 0.2-0.3% scalps — they cannot cover fees.
- Give positions room to develop: place stops beyond short-term noise (around -3%) and targets at meaningful heatmap resistance/liquidation zones (around +8%). Do not exit on small green or small red.

# Hard Risk Constraints

## Backend enforced
- Max positions: 2 Claw402 candidate instruments at the same time
- Max notional per position: 146 USDT (= equity 29 × 5.0x)
- Max margin usage: ≤100%
- Min order size: ≥12 USDT

## AI guided
- Leverage: every open position must use exactly 10x
- Risk/reward: ≥1:3.0
- Min confidence to open: ≥78

# Position Sizing

For every `open_long` or `open_short`, use the full max notional per position.
- Do not scale position_size_usd down by confidence.
- Do not open small probe positions.
- If the setup is not strong enough for full size, output `wait`.
- Do not use available_balance directly as position_size_usd.

# Output Format (Strictly Follow)

Use XML tags <reasoning> and <decision> to separate concise analysis from the decision JSON.

Direction must be data-driven: use `open_long` for confirmed upside structures and `open_short` for confirmed downside structures; never default to long-only or short-only behavior.

Evaluate both directions every cycle, but enter a side only when its own signals independently justify it. Never open a position just to balance the book — an unbalanced book beats a forced trade.

<reasoning>
Briefly state whether Claw402 ranking, Signal Lab, heatmap and candles agree; if data is missing or conflicting, explain why you wait.
</reasoning>

<decision>
```json
[
  {"symbol": "xyz:NVDA", "action": "open_long", "leverage": 10, "position_size_usd": 146, "stop_loss": 0, "take_profit": 0, "confidence": 85, "risk_usd": 0},
  {"symbol": "xyz:AAPL", "action": "open_short", "leverage": 10, "position_size_usd": 146, "stop_loss": 0, "take_profit": 0, "confidence": 85, "risk_usd": 0}
]
```
</decision>

## Field Requirements

- `action`: open_long | open_short | close_long | close_short | hold | wait
- `confidence`: 0-100; recommended ≥ 78 to open
- Required when opening: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd
- All numeric values must be calculated numbers, not formulas.
- JSON symbols must exactly match current candidates or existing positions; keep `xyz:` on XYZ instruments, and do not add `xyz:` or `USDT` to core crypto symbols.


--- User Prompt ---
Time: 2026-08-11 09:24:28 UTC | Period: #1 | Runtime: 0 minutes

Account: Equity 29.27 | Balance 29.27 (100.0%) | PnL +0.00% | Margin 0.0% | Positions 0

Current Positions: None

## Candidate Coins (10 coins)

### 1. xyz:SP500 (Vergex Signal)

=== xyz:SP500 Market Data ===

current_price = 7749.5000

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    7758.5000 7759.1000 7757.6000 7759.0000 55.97       
08-11 02:15    7759.0000 7759.7000 7757.0000 7759.1000 272.03      
08-11 02:30    7759.1000 7761.1000 7759.1000 7761.1000 127.88      
08-11 02:45    7761.2000 7761.3000 7760.1000 7760.1000 101.10      
08-11 03:00    7760.2000 7761.1000 7759.7000 7760.5000 34.65       
08-11 03:15    7760.5000 7763.6000 7760.1000 7763.5000 93.24       
08-11 03:30    7763.6000 7763.7000 7762.0000 7762.4000 35.90       
08-11 03:45    7762.3000 7762.3000 7760.9000 7760.9000 50.35       
08-11 04:00    7761.0000 7762.0000 7760.4000 7761.9000 67.24       
08-11 04:15    7762.0000 7762.2000 7761.0000 7762.0000 41.75       
08-11 04:30    7762.1000 7763.5000 7760.9000 7761.4000 292.59      
08-11 04:45    7761.5000 7761.9000 7761.3000 7761.3000 60.37       
08-11 05:00    7761.4000 7762.7000 7761.0000 7762.7000 40.12       
08-11 05:15    7762.7000 7763.3000 7757.9000 7758.9000 334.65      
08-11 05:30    7758.9000 7760.0000 7757.0000 7758.9000 131.29      
08-11 05:45    7758.9000 7759.1000 7754.5000 7754.6000 228.20      
08-11 06:00    7754.5000 7757.8000 7754.2000 7757.2000 172.23      
08-11 06:15    7757.3000 7757.3000 7748.7000 7748.7000 108.53      
08-11 06:30    7748.8000 7752.2000 7744.9000 7751.0000 116.41      
08-11 06:45    7750.8000 7751.2000 7746.2000 7746.7000 688.01      
08-11 07:00    7746.7000 7749.8000 7745.7000 7747.3000 857.41      
08-11 07:15    7747.2000 7747.8000 7744.0000 7746.9000 99.10       
08-11 07:30    7746.9000 7750.6000 7746.9000 7748.9000 84.45       
08-11 07:45    7749.0000 7751.6000 7747.8000 7750.7000 763.20      
08-11 08:00    7750.8000 7754.6000 7745.4000 7745.4000 258.05      
08-11 08:15    7745.4000 7747.1000 7742.9000 7745.9000 732.08      
08-11 08:30    7745.7000 7749.5000 7744.0000 7749.5000 489.87      
08-11 08:45    7749.4000 7750.8000 7748.3000 7749.2000 281.50      
08-11 09:00    7749.2000 7750.3000 7745.7000 7748.0000 131.33      
08-11 09:15    7748.2000 7751.8000 7748.2000 7749.5000 156.27        <- current



Vergex Claw402 Signals:
### xyz:SP500 (Vergex hip3_perp/SP500)
Ranking: rank=1 bias=bullish confidence=0.00 score=0.0000 category=stock
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 1

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 96.7 | book-wide trapped cost: support below (underwater shorts) $218.22M vs overhead above (underwater longs) $24.05M · longs P&L $9.76M / shorts P&L $-11.30M · trapped shorts below outweigh trapped longs above → net suppo...<truncated> |
| I Cost & Positioning | Value-area position | bullish | flat | 66.7 | price $7750 · POC $7704 (-0.6%) · value area $7343~$7795 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -4.8% vs weighted avg resistance +0.6% from price · overhead resistance is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bullish | medium | 86.7 | within ±15%: short liquidations above (squeeze fuel) $145.66M vs long liquidations below (flush fuel) $96.33M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $8066 (+4.1%) · 17% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 28.8% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 96.7 | within ±15%: support below (trapped shorts cover) $213.06M @ $7524 vs resistance above (trapped longs break even) $24.05M @ $7795 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:SP500/riskbins?chain=mainnet)

### 2. xyz:SPCX (Vergex Signal)

=== xyz:SPCX Market Data ===

current_price = 136.9200

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    136.5600  136.8100  136.3200  136.3400  2853.46     
08-11 02:15    136.3200  136.3300  136.0800  136.2600  1996.85     
08-11 02:30    136.2600  136.6700  136.2400  136.6600  2052.49     
08-11 02:45    136.6400  136.6900  136.5300  136.5500  1602.90     
08-11 03:00    136.5300  136.5800  136.4700  136.4800  785.70      
08-11 03:15    136.4800  136.7000  136.4700  136.6500  1025.59     
08-11 03:30    136.6400  136.6800  136.4500  136.4600  2180.28     
08-11 03:45    136.4500  136.5900  136.4400  136.5200  1639.04     
08-11 04:00    136.5200  136.7100  136.5200  136.6900  1258.06     
08-11 04:15    136.7000  137.0500  136.6600  137.0500  2242.99     
08-11 04:30    137.0400  137.5800  137.0100  137.2100  6033.73     
08-11 04:45    137.2300  137.3100  137.1300  137.1600  4022.15     
08-11 05:00    137.1500  137.1800  136.9200  137.1100  1901.83     
08-11 05:15    137.1000  137.1200  136.7600  136.8500  1859.86     
08-11 05:30    136.8400  137.0500  136.7700  136.9100  998.21      
08-11 05:45    136.9100  137.1100  136.9100  136.9900  959.96      
08-11 06:00    137.0000  137.0200  136.8800  136.9900  1586.28     
08-11 06:15    136.9900  137.0100  136.7200  136.7600  1872.84     
08-11 06:30    136.8000  136.9800  136.6800  136.8600  2906.77     
08-11 06:45    136.8600  136.8800  136.5300  136.7100  4128.72     
08-11 07:00    136.7100  136.7900  136.5600  136.7900  3953.72     
08-11 07:15    136.7900  137.2400  136.7900  137.1600  4512.95     
08-11 07:30    137.1900  137.2800  137.0400  137.0400  3097.96     
08-11 07:45    137.0500  137.4200  137.0400  137.2600  4194.70     
08-11 08:00    137.2200  137.6200  136.3100  136.3400  6248.16     
08-11 08:15    136.3700  136.9100  136.3600  136.6900  8490.22     
08-11 08:30    136.6900  136.8000  136.4600  136.7900  2957.17     
08-11 08:45    136.8000  137.1000  136.6700  136.8700  5093.03     
08-11 09:00    136.8800  136.8800  136.4000  136.4700  1797.05     
08-11 09:15    136.4700  136.9800  136.4600  136.9200  3000.20       <- current



Vergex Claw402 Signals:
### xyz:SPCX (Vergex hip3_perp/SPCX)
Ranking: rank=2 bias=bullish confidence=0.00 score=0.0000 category=stock
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 1.49

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 83.3 | book-wide trapped cost: support below (underwater shorts) $85.84M vs overhead above (underwater longs) $14.84M · longs P&L $6.49M / shorts P&L $-3.69M · trapped shorts below outweigh trapped longs above → net support...<truncated> |
| I Cost & Positioning | Value-area position | bullish | strong | 96.7 | price $136.920 · POC $123.342 (-9.9%) · value area $123.342~$137.719 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -8.0% vs weighted avg resistance +7.9% from price · trapped support and resistance sit at a similar cost-weighted distance |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bullish | strong | 93.3 | within ±15%: short liquidations above (squeeze fuel) $13.11M vs long liquidations below (flush fuel) $5.38M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $121.745 (-11.1%) · 13% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 1.6% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 90 | within ±15%: support below (trapped shorts cover) $79.25M @ $123.342 vs resistance above (trapped longs break even) $11.14M @ $137.719 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:SPCX/riskbins?chain=mainnet)

### 3. xyz:BRENTOIL (Vergex Signal)

=== xyz:BRENTOIL Market Data ===

current_price = 88.7410

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    86.9640   86.9840   86.8460   86.9730   5773.98     
08-11 02:15    86.9530   87.0560   86.8370   86.8780   2035.56     
08-11 02:30    86.8780   86.8780   86.7560   86.7880   5463.77     
08-11 02:45    86.7950   87.0290   86.7710   86.9280   6544.28     
08-11 03:00    86.9630   86.9850   86.8220   86.8510   5399.72     
08-11 03:15    86.8660   86.9610   86.8200   86.8240   6110.65     
08-11 03:30    86.8300   86.8300   86.6750   86.7310   17875.04    
08-11 03:45    86.7180   86.7770   86.6980   86.6980   3868.12     
08-11 04:00    86.6980   86.7270   86.6000   86.6060   12293.62    
08-11 04:15    86.6060   86.6430   86.5340   86.5950   19029.75    
08-11 04:30    86.5650   86.8480   86.5380   86.8320   3332.49     
08-11 04:45    86.8330   86.9010   86.7470   86.8160   2665.88     
08-11 05:00    86.8080   86.9540   86.7880   86.9210   5257.94     
08-11 05:15    86.9220   87.1510   86.8850   86.9890   32818.42    
08-11 05:30    86.9600   87.1180   86.7250   86.8480   8694.57     
08-11 05:45    86.8460   87.0680   86.8390   86.9650   9351.93     
08-11 06:00    86.9530   87.0790   86.8220   87.0440   9346.30     
08-11 06:15    87.0610   87.2920   87.0010   87.2870   11355.05    
08-11 06:30    87.2950   88.0840   87.2290   88.0280   41878.22    
08-11 06:45    88.0280   88.2500   87.9350   87.9730   28703.09    
08-11 07:00    87.9840   88.1800   87.7790   88.1330   22343.03    
08-11 07:15    88.1280   88.5330   88.0520   88.2320   26796.75    
08-11 07:30    88.2350   88.4890   88.1690   88.1800   17907.58    
08-11 07:45    88.2160   88.5580   88.2160   88.5200   17577.39    
08-11 08:00    88.4950   88.7100   88.1730   88.6810   25149.47    
08-11 08:15    88.6770   88.8430   88.6000   88.7600   18576.58    
08-11 08:30    88.7530   88.8430   88.5100   88.5370   32218.72    
08-11 08:45    88.5450   88.8080   88.5330   88.7810   18984.99    
08-11 09:00    88.7950   88.8370   88.5840   88.8090   27178.98    
08-11 09:15    88.8090   88.8380   88.6570   88.7410   20508.27      <- current



Vergex Claw402 Signals:
### xyz:BRENTOIL (Vergex hip3_perp/BRENTOIL)
Ranking: rank=3 bias=bullish confidence=0.00 score=0.0000 category=commodity
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 1.25

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 80 | book-wide trapped cost: support below (underwater shorts) $111.03M vs overhead above (underwater longs) $23.52M · longs P&L $3.24M / shorts P&L $-4.33M · trapped shorts below outweigh trapped longs above → net suppor...<truncated> |
| I Cost & Positioning | Value-area position | bullish | medium | 90 | price $88.746 · POC $86.158 (-2.9%) · value area $82.016~$87.193 · above the value area (acceptance higher → bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -4.3% vs weighted avg resistance +11.5% from price · trapped support is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bullish | strong | 96.7 | within ±15%: short liquidations above (squeeze fuel) $37.07M vs long liquidations below (flush fuel) $13.02M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $94.441 (+6.4%) · 58% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 3.3% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 76.7 | within ±15%: support below (trapped shorts cover) $108.98M @ $86.158 vs resistance above (trapped longs break even) $23.30M @ $100.653 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:BRENTOIL/riskbins?chain=mainnet)

### 4. xyz:MSFT (Vergex Signal)

=== xyz:MSFT Market Data ===

current_price = 504.7300

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    505.7900  505.9500  505.6500  505.6500  8.25        
08-11 02:15    505.8000  506.0700  505.8000  506.0100  24.86       
08-11 02:30    506.0700  506.0900  506.0000  506.0000  3.13        
08-11 02:45    506.1200  506.1900  506.1200  506.1900  4.08        
08-11 03:00    506.1000  506.1300  505.8900  505.9000  4.83        
08-11 03:15    505.9200  506.2800  505.9200  506.2800  6.76        
08-11 03:30    506.4200  506.4300  506.1600  506.2700  59.72       
08-11 03:45    506.0800  506.3000  505.8800  505.9100  137.19      
08-11 04:00    505.9100  505.9500  505.6400  505.9100  63.47       
08-11 04:15    505.8000  505.8400  505.8000  505.8400  1.39        
08-11 04:30    505.7400  505.8800  505.5000  505.6600  197.78      
08-11 04:45    505.7000  505.7900  505.5800  505.7000  40.35       
08-11 05:00    505.6300  505.8400  505.6300  505.8400  5.47        
08-11 05:15    505.7600  505.9300  505.6900  505.9300  78.85       
08-11 05:30    505.8300  505.8300  505.4800  505.4800  64.10       
08-11 05:45    505.4300  505.5700  505.0300  505.3300  102.46      
08-11 06:00    505.3900  505.3900  505.2300  505.3100  8.20        
08-11 06:15    505.3000  505.3000  504.8200  504.8300  79.46       
08-11 06:30    504.8100  504.8700  504.3800  504.7300  84.65       
08-11 06:45    504.6700  504.7000  504.0700  504.3000  474.35      
08-11 07:00    504.3000  504.5500  504.2100  504.2800  123.46      
08-11 07:15    504.3400  504.5600  504.1800  504.4700  50.37       
08-11 07:30    504.5900  504.8500  504.5900  504.7200  15.60       
08-11 07:45    504.6200  504.6500  504.5000  504.5800  6.80        
08-11 08:00    504.4900  504.8500  503.0300  504.1600  563.74      
08-11 08:15    504.1700  504.8800  503.7800  504.5600  596.57      
08-11 08:30    504.4300  504.6400  504.1000  504.2600  159.11      
08-11 08:45    504.2300  504.5600  504.1400  504.4400  323.76      
08-11 09:00    504.3400  504.6600  504.1000  504.3800  275.78      
08-11 09:15    504.4000  504.7300  504.3100  504.7300  37.85         <- current



Vergex Claw402 Signals:
### xyz:MSFT (Vergex hip3_perp/MSFT)
Ranking: rank=4 bias=bullish confidence=0.00 score=0.0000 category=stock
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 1.27

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 100 | book-wide trapped cost: support below (underwater shorts) $25.60M vs overhead above (underwater longs) $1.30M · longs P&L $1.68M / shorts P&L $-3.36M · trapped shorts below outweigh trapped longs above → net support ...<truncated> |
| I Cost & Positioning | Value-area position | bullish | flat | 70 | price $504.590 · POC $495.760 (-1.7%) · value area $460.438~$507.533 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -10.3% vs weighted avg resistance +0.6% from price · overhead resistance is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bullish | strong | 90 | within ±15%: short liquidations above (squeeze fuel) $6.09M vs long liquidations below (flush fuel) $2.53M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $513.420 (+1.8%) · 35% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 7.1% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 100 | within ±15%: support below (trapped shorts cover) $17.99M @ $483.986 vs resistance above (trapped longs break even) $1.30M @ $507.533 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:MSFT/riskbins?chain=mainnet)

### 5. xyz:GOLD (Vergex Signal)

=== xyz:GOLD Market Data ===

current_price = 4375.9000

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    4432.8000 4437.8000 4425.6000 4437.7000 239.79      
08-11 02:15    4437.7000 4437.9000 4430.0000 4432.6000 236.00      
08-11 02:30    4431.9000 4432.7000 4417.7000 4422.4000 105.78      
08-11 02:45    4422.3000 4422.4000 4411.7000 4413.9000 202.19      
08-11 03:00    4413.9000 4418.0000 4413.5000 4417.1000 85.06       
08-11 03:15    4417.1000 4424.4000 4416.8000 4423.4000 89.74       
08-11 03:30    4423.4000 4424.7000 4418.3000 4418.5000 125.41      
08-11 03:45    4419.8000 4420.8000 4416.9000 4420.0000 74.36       
08-11 04:00    4420.0000 4422.0000 4415.4000 4415.6000 79.19       
08-11 04:15    4415.6000 4418.2000 4413.6000 4417.1000 145.61      
08-11 04:30    4417.1000 4422.5000 4411.3000 4413.4000 207.06      
08-11 04:45    4413.4000 4414.1000 4406.4000 4412.2000 198.30      
08-11 05:00    4412.2000 4412.2000 4403.0000 4404.6000 144.92      
08-11 05:15    4404.6000 4404.7000 4393.3000 4394.7000 446.30      
08-11 05:30    4394.8000 4394.8000 4375.3000 4379.9000 982.33      
08-11 05:45    4379.8000 4380.7000 4370.0000 4376.9000 922.38      
08-11 06:00    4376.8000 4384.4000 4375.0000 4375.2000 352.73      
08-11 06:15    4375.3000 4376.7000 4363.3000 4365.3000 522.14      
08-11 06:30    4365.3000 4373.4000 4363.0000 4366.3000 814.91      
08-11 06:45    4366.4000 4369.7000 4363.0000 4368.3000 113.86      
08-11 07:00    4368.3000 4368.8000 4363.0000 4365.2000 398.14      
08-11 07:15    4365.2000 4366.9000 4360.2000 4364.0000 545.90      
08-11 07:30    4364.0000 4366.1000 4360.4000 4360.5000 343.92      
08-11 07:45    4360.5000 4362.8000 4358.6000 4358.8000 792.12      
08-11 08:00    4358.8000 4368.3000 4358.8000 4367.1000 471.22      
08-11 08:15    4367.2000 4371.3000 4361.6000 4371.2000 282.36      
08-11 08:30    4371.9000 4377.7000 4371.7000 4377.6000 91.93       
08-11 08:45    4377.5000 4378.0000 4373.4000 4373.4000 76.05       
08-11 09:00    4373.3000 4375.0000 4371.1000 4375.0000 124.71      
08-11 09:15    4375.0000 4377.6000 4374.1000 4375.9000 27.79         <- current



Vergex Claw402 Signals:
### xyz:GOLD (Vergex hip3_perp/GOLD)
Ranking: rank=5 bias=bullish confidence=0.00 score=0.0000 category=commodity
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 0.92

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 93.3 | book-wide trapped cost: support below (underwater shorts) $158.40M vs overhead above (underwater longs) $22.89M · longs P&L $5.13M / shorts P&L $-7.42M · trapped shorts below outweigh trapped longs above → net suppor...<truncated> |
| I Cost & Positioning | Value-area position | bullish | strong | 93.3 | price $4376 · POC $4146 (-5.2%) · value area $4044~$4299 · above the value area (acceptance higher → bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -4.7% vs weighted avg resistance +2.4% from price · trapped support and resistance sit at a similar cost-weighted distance |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | flat | 53.3 | within ±15%: short liquidations above (squeeze fuel) $23.51M vs long liquidations below (flush fuel) $46.83M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $3840 (-12.2%) · 15% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 4.1% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 93.3 | within ±15%: support below (trapped shorts cover) $158.40M @ $4197 vs resistance above (trapped longs break even) $22.23M @ $4401 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:GOLD/riskbins?chain=mainnet)

### 6. xyz:CL (Vergex Signal)

=== xyz:CL Market Data ===

current_price = 83.7660

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    81.8890   81.8890   81.7710   81.8720   6501.75     
08-11 02:15    81.8540   81.9150   81.7500   81.7850   11255.66    
08-11 02:30    81.7730   81.7730   81.6620   81.6760   10188.25    
08-11 02:45    81.6760   81.8840   81.6700   81.7790   28480.78    
08-11 03:00    81.8210   81.8580   81.7030   81.7300   6842.53     
08-11 03:15    81.7340   81.8520   81.6970   81.7050   3400.02     
08-11 03:30    81.7050   81.7050   81.5320   81.5590   53054.25    
08-11 03:45    81.5590   81.6200   81.5400   81.5580   22520.04    
08-11 04:00    81.5590   81.5630   81.4880   81.5090   37674.63    
08-11 04:15    81.5090   81.5320   81.4410   81.5070   14976.24    
08-11 04:30    81.4920   81.7240   81.4490   81.7150   4160.96     
08-11 04:45    81.7220   81.7950   81.6470   81.7160   6123.89     
08-11 05:00    81.7170   81.8540   81.7060   81.8390   9537.44     
08-11 05:15    81.8460   82.0170   81.8080   81.8690   12816.78    
08-11 05:30    81.8560   81.9970   81.6280   81.7230   15671.18    
08-11 05:45    81.7350   81.9640   81.7280   81.9080   7179.47     
08-11 06:00    81.9060   81.9840   81.7680   81.9730   24871.58    
08-11 06:15    81.9670   82.2380   81.9380   82.2380   25631.58    
08-11 06:30    82.2410   83.0700   82.1780   83.0180   60369.57    
08-11 06:45    83.0130   83.1250   82.8660   82.8660   34328.52    
08-11 07:00    82.8660   83.0470   82.6410   82.9960   32604.20    
08-11 07:15    82.9970   83.3100   82.9250   83.0910   80796.25    
08-11 07:30    83.0950   83.3200   83.0300   83.0350   24193.78    
08-11 07:45    83.0480   83.4200   83.0480   83.3820   21272.10    
08-11 08:00    83.3810   83.6270   83.1310   83.5680   43047.21    
08-11 08:15    83.5680   83.7290   83.5350   83.6730   87171.46    
08-11 08:30    83.6700   83.7440   83.5010   83.5100   69393.37    
08-11 08:45    83.5180   83.8280   83.5160   83.8220   33207.54    
08-11 09:00    83.8220   83.9160   83.6430   83.8760   30068.40    
08-11 09:15    83.8710   83.9000   83.6950   83.7660   25601.30      <- current



Vergex Claw402 Signals:
### xyz:CL (Vergex hip3_perp/CL)
Ranking: rank=6 bias=bullish confidence=0.00 score=0.0000 category=commodity
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 0.36

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 76.7 | book-wide trapped cost: support below (underwater shorts) $82.06M vs overhead above (underwater longs) $18.10M · longs P&L $4.25M / shorts P&L $-3.17M · trapped shorts below outweigh trapped longs above → net support...<truncated> |
| I Cost & Positioning | Value-area position | bullish | weak | 73.3 | price $83.786 · POC $82.320 (-1.8%) · value area $77.432~$84.275 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -6.0% vs weighted avg resistance +1.9% from price · overhead resistance is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | weak | 43.3 | within ±15%: short liquidations above (squeeze fuel) $12.04M vs long liquidations below (flush fuel) $38.15M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $77.432 (-7.6%) · 50% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 2.9% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 73.3 | within ±15%: support below (trapped shorts cover) $82.03M @ $78.410 vs resistance above (trapped longs break even) $17.82M @ $84.275 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:CL/riskbins?chain=mainnet)

### 7. xyz:NVDA (Vergex Signal)

=== xyz:NVDA Market Data ===

current_price = 219.7300

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    219.0600  219.2900  218.8600  219.1000  233.66      
08-11 02:15    219.1000  219.2700  219.0700  219.2700  426.39      
08-11 02:30    219.2600  219.3400  219.1500  219.3000  370.14      
08-11 02:45    219.2900  219.2900  219.0000  219.0100  471.03      
08-11 03:00    219.0000  219.0400  218.9200  219.0200  321.15      
08-11 03:15    219.0100  219.1000  218.9400  219.0800  112.79      
08-11 03:30    219.0700  219.2200  219.0100  219.1900  1434.63     
08-11 03:45    219.1800  219.2700  219.1200  219.1400  193.69      
08-11 04:00    219.1300  219.2600  219.1300  219.2400  844.04      
08-11 04:15    219.2400  219.2800  219.2300  219.2800  204.86      
08-11 04:30    219.2800  219.6300  219.2700  219.5100  1488.87     
08-11 04:45    219.5000  219.6000  219.2500  219.2800  514.66      
08-11 05:00    219.2700  219.4500  219.2000  219.4500  254.35      
08-11 05:15    219.4200  219.4400  218.9400  218.9400  353.98      
08-11 05:30    218.9400  219.1700  218.8700  219.1200  488.66      
08-11 05:45    219.1200  219.1700  219.0700  219.0700  258.98      
08-11 06:00    219.0800  219.2700  219.0500  219.2300  397.77      
08-11 06:15    219.2300  219.2600  218.8900  218.9000  519.05      
08-11 06:30    218.9000  218.9400  218.6500  218.8700  496.78      
08-11 06:45    218.8700  218.8800  218.5400  218.8000  715.77      
08-11 07:00    218.8200  218.8700  218.6100  218.7400  354.63      
08-11 07:15    218.7300  218.7300  218.5000  218.6200  398.65      
08-11 07:30    218.6200  218.9400  218.6200  218.7600  282.06      
08-11 07:45    218.7900  218.8700  218.6100  218.8500  350.00      
08-11 08:00    218.8800  219.1600  218.6200  218.7400  2408.16     
08-11 08:15    218.7400  218.8400  218.5500  218.6500  1389.54     
08-11 08:30    218.6400  218.8100  218.3800  218.8100  656.85      
08-11 08:45    218.8000  219.4200  218.8000  219.1000  1164.19     
08-11 09:00    219.1000  219.2000  218.8300  219.1500  1290.34     
08-11 09:15    219.1600  219.8600  219.1600  219.7300  1030.06       <- current



Vergex Claw402 Signals:
### xyz:NVDA (Vergex hip3_perp/NVDA)
Ranking: rank=7 bias=bullish confidence=0.00 score=0.0000 category=stock
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 0.41

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 86.7 | book-wide trapped cost: support below (underwater shorts) $67.89M vs overhead above (underwater longs) $10.34M · longs P&L $3.62M / shorts P&L $-3.32M · trapped shorts below outweigh trapped longs above → net support...<truncated> |
| I Cost & Positioning | Value-area position | bullish | medium | 86.7 | price $219.760 · POC $210.786 (-4.1%) · value area $197.967~$215.914 · above the value area (acceptance higher → bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -4.7% vs weighted avg resistance +0.8% from price · overhead resistance is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | medium | 16.7 | within ±15%: short liquidations above (squeeze fuel) $668.1K vs long liquidations below (flush fuel) $4.36M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $210.786 (-4.1%) · 27% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 1.4% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 86.7 | within ±15%: support below (trapped shorts cover) $65.48M @ $210.786 vs resistance above (trapped longs break even) $10.34M @ $221.042 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:NVDA/riskbins?chain=mainnet)

### 8. xyz:SILVER (Vergex Signal)

=== xyz:SILVER Market Data ===

current_price = 64.8960

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    66.3520   66.3640   66.1080   66.3340   7724.87     
08-11 02:15    66.3420   66.3690   66.2560   66.3310   7771.89     
08-11 02:30    66.3270   66.3360   65.7820   65.9220   10480.07    
08-11 02:45    65.9140   65.9290   65.6190   65.7380   16658.33    
08-11 03:00    65.7450   65.8970   65.7450   65.8350   9571.79     
08-11 03:15    65.8360   65.9110   65.7940   65.8490   13956.67    
08-11 03:30    65.8350   65.9760   65.8350   65.8350   5304.69     
08-11 03:45    65.8360   65.8840   65.8010   65.8090   3458.39     
08-11 04:00    65.8020   65.8810   65.7600   65.8100   4152.26     
08-11 04:15    65.8190   65.8490   65.7200   65.7530   13505.38    
08-11 04:30    65.7530   65.8470   65.4940   65.5200   19068.82    
08-11 04:45    65.5200   65.5740   65.4130   65.5740   23146.55    
08-11 05:00    65.5740   65.5780   65.3920   65.4230   10510.62    
08-11 05:15    65.4240   65.4240   65.1140   65.1340   17849.10    
08-11 05:30    65.1110   65.1170   64.5330   64.7750   73596.66    
08-11 05:45    64.7790   64.8220   64.5480   64.8220   18823.79    
08-11 06:00    64.8250   65.0220   64.7270   64.7450   19207.89    
08-11 06:15    64.7330   64.8420   64.3720   64.4150   19638.44    
08-11 06:30    64.4110   64.5780   64.2940   64.4870   18250.99    
08-11 06:45    64.4860   64.6030   64.4490   64.5220   5461.37     
08-11 07:00    64.5360   64.6310   64.4920   64.5600   5753.41     
08-11 07:15    64.5650   64.5810   64.3800   64.4600   9054.69     
08-11 07:30    64.4580   64.5730   64.3500   64.3940   18239.08    
08-11 07:45    64.4060   64.5130   64.3750   64.4480   13856.24    
08-11 08:00    64.4530   64.7590   64.4530   64.6800   8751.27     
08-11 08:15    64.6750   64.7380   64.5720   64.7000   8139.94     
08-11 08:30    64.7000   64.8810   64.6820   64.8470   13736.24    
08-11 08:45    64.8580   64.9260   64.7060   64.7060   10852.88    
08-11 09:00    64.7040   64.8590   64.6560   64.8100   41427.97    
08-11 09:15    64.8180   64.9230   64.8110   64.8960   4623.37       <- current



Vergex Claw402 Signals:
### xyz:SILVER (Vergex hip3_perp/SILVER)
Ranking: rank=8 bias=bullish confidence=0.00 score=0.0000 category=commodity
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: Medium · some positions missing cost/liquidation price
- Composite Z: 0.43

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | medium | 73.3 | book-wide trapped cost: support below (underwater shorts) $60.90M vs overhead above (underwater longs) $14.91M · longs P&L $1.46M / shorts P&L $-2.12M · trapped shorts below outweigh trapped longs above → net support...<truncated> |
| I Cost & Positioning | Value-area position | bullish | medium | 76.7 | price $64.888 · POC $62.238 (-4.1%) · value area $59.210~$66.024 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -5.2% vs weighted avg resistance +11.2% from price · trapped support is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | weak | 36.7 | within ±15%: short liquidations above (squeeze fuel) $4.69M vs long liquidations below (flush fuel) $16.11M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $62.995 (-2.9%) · 33% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 7.5% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | medium | 83.3 | within ±15%: support below (trapped shorts cover) $60.90M @ $62.238 vs resistance above (trapped longs break even) $11.91M @ $74.351 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:SILVER/riskbins?chain=mainnet)

### 9. xyz:CXMT (Vergex Signal)

=== xyz:CXMT Market Data ===

current_price = 7.2846

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    7.2900    7.2908    7.2700    7.2844    16506.40    
08-11 02:15    7.2843    7.2900    7.2727    7.2866    11259.40    
08-11 02:30    7.2847    7.3155    7.2760    7.3087    25704.90    
08-11 02:45    7.3091    7.4301    7.2958    7.3364    133962.20   
08-11 03:00    7.3336    7.3381    7.3002    7.3209    33229.50    
08-11 03:15    7.3248    7.3629    7.3095    7.3232    27832.70    
08-11 03:30    7.3231    7.3869    7.3118    7.3700    26120.00    
08-11 03:45    7.3686    7.3704    7.3527    7.3612    4354.50     
08-11 04:00    7.3612    7.3612    7.3381    7.3443    9771.20     
08-11 04:15    7.3446    7.3697    7.3397    7.3627    8605.70     
08-11 04:30    7.3601    7.3661    7.3600    7.3655    6609.90     
08-11 04:45    7.3655    7.3758    7.3634    7.3635    13667.30    
08-11 05:00    7.3606    7.4852    7.3320    7.3464    122203.30   
08-11 05:15    7.3494    7.3756    7.3343    7.3450    53644.10    
08-11 05:30    7.3485    7.3626    7.3329    7.3590    14405.60    
08-11 05:45    7.3613    7.3920    7.3500    7.3781    33342.10    
08-11 06:00    7.3750    7.3750    7.3127    7.3166    58817.00    
08-11 06:15    7.3150    7.4500    7.3000    7.3053    153491.20   
08-11 06:30    7.3053    7.3923    7.2741    7.2862    154023.50   
08-11 06:45    7.2862    7.3149    7.2823    7.3005    26924.40    
08-11 07:00    7.3005    7.3005    7.2700    7.2823    38794.50    
08-11 07:15    7.2822    7.2929    7.2805    7.2898    17813.30    
08-11 07:30    7.2863    7.2948    7.2801    7.2906    17608.10    
08-11 07:45    7.2927    7.3020    7.2899    7.2945    27498.50    
08-11 08:00    7.2955    7.2976    7.2800    7.2844    32022.40    
08-11 08:15    7.2841    7.2920    7.2800    7.2804    19297.50    
08-11 08:30    7.2804    7.2877    7.2800    7.2805    13998.00    
08-11 08:45    7.2840    7.2872    7.2800    7.2863    5052.90     
08-11 09:00    7.2839    7.2954    7.2800    7.2866    6777.20     
08-11 09:15    7.2902    7.2927    7.2801    7.2846    10017.40      <- current



Vergex Claw402 Signals:
### xyz:CXMT (Vergex hip3_perp/CXMT)
Ranking: rank=9 bias=bullish confidence=0.00 score=0.0000 category=stock
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 0.78

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | weak | 66.7 | book-wide trapped cost: support below (underwater shorts) $26.63M vs overhead above (underwater longs) $12.71M · longs P&L $1.61M / shorts P&L $-2.77M · trapped shorts below outweigh trapped longs above → net support...<truncated> |
| I Cost & Positioning | Value-area position | bullish | strong | 100 | price $7.281 · POC $6.474 (-11.1%) · value area $6.474~$7.324 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -9.9% vs weighted avg resistance +3.7% from price · overhead resistance is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | weak | 70 | within ±15%: short liquidations above (squeeze fuel) $2.25M vs long liquidations below (flush fuel) $2.53M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $6.474 (-11.1%) · 20% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 0.2% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | weak | 63.3 | within ±15%: support below (trapped shorts cover) $26.55M @ $6.474 vs resistance above (trapped longs break even) $12.71M @ $7.409 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:CXMT/riskbins?chain=mainnet)

### 10. xyz:SMSN (Vergex Signal)

=== xyz:SMSN Market Data ===

current_price = 168.2900

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
08-11 02:00    168.5800  169.3600  167.6900  168.0000  3040.61     
08-11 02:15    167.9800  168.2600  166.8300  167.4200  3858.14     
08-11 02:30    167.4000  168.8400  167.3800  168.8300  2947.79     
08-11 02:45    168.8400  169.1100  168.5400  168.7800  1887.41     
08-11 03:00    168.7300  169.4700  168.6000  169.0300  5104.47     
08-11 03:15    169.0400  170.8600  168.6200  170.6800  4634.33     
08-11 03:30    170.7600  171.1800  170.2700  170.3200  2491.29     
08-11 03:45    170.3900  170.7900  169.8200  170.2500  5753.02     
08-11 04:00    170.2900  171.5500  170.2900  171.1400  4563.33     
08-11 04:15    171.1300  171.3300  170.6400  171.1800  1794.22     
08-11 04:30    171.1800  171.5300  170.6200  170.7400  2754.15     
08-11 04:45    170.7400  171.1100  170.4800  170.6300  1506.55     
08-11 05:00    170.6000  170.9200  170.1000  170.8500  3472.26     
08-11 05:15    170.9200  170.9200  168.7100  168.7500  4809.33     
08-11 05:30    168.7200  170.1200  168.5700  169.4300  4368.10     
08-11 05:45    169.4600  170.0900  169.3200  169.4900  4475.80     
08-11 06:00    169.5200  169.6700  169.0600  169.2400  2976.52     
08-11 06:15    169.2600  169.5500  168.4100  169.2400  7785.91     
08-11 06:30    169.2600  169.2600  168.4000  168.4500  2550.34     
08-11 06:45    168.4500  168.4900  168.0900  168.4000  1166.96     
08-11 07:00    168.3200  168.5400  168.2400  168.4400  1230.39     
08-11 07:15    168.4500  168.6500  168.3300  168.6400  733.54      
08-11 07:30    168.6600  168.7700  168.5600  168.6000  2031.59     
08-11 07:45    168.6000  168.6300  168.2000  168.4400  3853.03     
08-11 08:00    168.4400  169.0100  168.1900  168.2300  3539.48     
08-11 08:15    168.2000  168.3300  168.1100  168.1100  1511.94     
08-11 08:30    168.1200  168.3200  167.6800  168.2500  3208.25     
08-11 08:45    168.2500  168.5100  168.2300  168.3700  1887.61     
08-11 09:00    168.3600  168.5100  167.5900  167.8100  2266.97     
08-11 09:15    167.8200  168.2900  167.8200  168.2900  126.32        <- current



Vergex Claw402 Signals:
### xyz:SMSN (Vergex hip3_perp/SMSN)
Ranking: rank=10 bias=bullish confidence=0.00 score=0.0000 category=stock
#### Signal Lab
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 0.33

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | weak | 60 | book-wide trapped cost: support below (underwater shorts) $14.59M vs overhead above (underwater longs) $7.22M · longs P&L $58.3K / shorts P&L $641.4K · trapped shorts below outweigh trapped longs above → net support ...<truncated> |
| I Cost & Positioning | Value-area position | bullish | flat | 63.3 | price $168.290 · POC $167.308 (-0.6%) · value area $161.418~$171.235 · in the upper value area (leaning bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -1.1% vs weighted avg resistance +12.6% from price · trapped support is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | flat | 60 | within ±15%: short liquidations above (squeeze fuel) $3.97M vs long liquidations below (flush fuel) $5.88M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $155.528 (-7.6%) · 21% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 0.2% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | weak | 70 | within ±15%: support below (trapped shorts cover) $14.59M @ $167.308 vs resistance above (trapped longs break even) $5.91M @ $190.869 |

Cost/Liquidation Heatmap: unavailable (vergex token rejected (401) for https://vergex.trade/api/v1/data-intelligence/markets/hip3_perp/xyz:SMSN/riskbins?chain=mainnet)


---

Now please analyze briefly and output the decision JSON.


--- Assistant Response ---
<reasoning>
All 10 candidates show bullish bias in Signal Lab, but I need to evaluate whether the signals, heatmap, and candles agree strongly enough to open a position.

Key observations:
- **Heatmap unavailable for ALL symbols** (401 error on all riskbins endpoints). This is a critical data gap - I cannot confirm cost/liquidation clusters for stop/target placement as required by my decision framework.
- **Signal Lab present for all symbols** with bullish bias, but confidence scores are 0.00 in the ranking (though Signal Lab shows "High" confidence for most).
- **Candles show mixed picture**: 
  - BRENTOIL and CL show strong uptrends (higher lows, higher highs)
  - GOLD shows a pullback from highs
  - SILVER shows a sharp decline then stabilization
  - SP500, MSFT, NVDA, SMSN are range-bound
  - SPCX shows a strong push up then slight pullback
  - CXMT shows volatile consolidation

The strongest candidates by Signal Lab composite Z:
1. SPCX (Z=1.49) - bullish, strong value-area and LFA signals
2. MSFT (Z=1.27) - bullish, strong CGO and cost walls
3. BRENTOIL (Z=1.25) - bullish, strong LFA, above value area

However, the **heatmap is completely unavailable** for all symbols. My rules state: "Open only when Signal Lab, heatmap and raw candles broadly agree; wait when key data is missing or contradictory." The heatmap is a key data source for stop placement and target zones, and it's missing for every candidate.

Additionally, several signals show internal contradictions:
- GOLD: CGO bullish but LFA bearish
- CL: CGO bullish but LFA bearish
- NVDA: CGO bullish but LFA bearish (medium strength)
- SILVER: CGO bullish but LFA bearish
- CXMT: CGO bullish but LFA bearish
- SMSN: CGO bullish but LFA bearish

The only symbols with consistent bullish LFA are SP500, SPCX, and BRENTOIL. But without heatmap confirmation, I cannot properly place stops and targets.

Given the missing heatmap data across all candidates, the contradictory LFA signals on most, and the requirement for data agreement across all three sources, I should **wait** this cycle rather than force a trade with incomplete information.
</reasoning>

<decision>
```json
[
  {"symbol": "xyz:SP500", "action": "wait", "leverage": 10, "position_size_usd": 0, "stop_loss": 0, "take_profit": 0, "confidence": 0, "risk_usd": 0}
]
```
</decision>
