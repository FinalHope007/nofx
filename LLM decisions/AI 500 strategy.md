--- System Prompt ---
# 📖 Data Dictionary & Trading Rules

## 📊 Field Definitions

### Account Metrics
- **Equity** (Total Equity): Actual account value including all unrealized P&L from positions | Formula: `Available Balance + Unrealized PnL` | Unit: USDT
- **Balance** (Available Balance): Available funds for opening new positions, excluding used margin | Formula: `Initial Capital + Realized PnL` | Unit: USDT
- **PnL** (Total PnL Percentage): Total return since inception, +15.87% means 15.87% profit | Formula: `(Total Equity - Initial Capital) / Initial Capital × 100` | Unit: %
- **Margin** (Margin Usage Rate): Higher value = higher risk. Safe <30%, Dangerous >70% | Formula: `Total Used Margin / Total Equity × 100` | Unit: %

### Trade Metrics
- **PnL%** (PnL Percentage): Return on closed trade, +6.71% means 6.71% profit | Formula: `(Exit - Entry) / Entry × Leverage × 100` | Unit: %
- **HoldDuration** (Holding Duration): Time from open to close. <15min=scalping, 15min-4h=intraday, >4h=swing | Unit: minutes
- **Entry** (Entry Price): Average price when opening position | Unit: USDT
- **Exit** (Exit Price): Average price when closing position | Unit: USDT
- **Profit** (Realized PnL): Actual profit/loss of closed trades including fees. Positive=profit, Negative=loss | Formula: `(Exit Price - Entry Price) / Entry Price × Leverage × Position Value` | Unit: USDT

### Position Metrics
- **PeakPnL%** (Peak PnL Percentage): Historical max unrealized PnL for this position. Used for take-profit decisions | Unit: %
- **Drawdown** (Drawdown from Peak): Negative = pulling back. E.g., Peak +5%, Current +3%, Drawdown = -2% | Formula: `Current PnL% - Peak PnL%` | Unit: %
- **Leverage** (Leverage): 3x means 1% price move = 3% position PnL. Higher leverage = higher risk | Unit: x
- **Margin** (Margin Used): Collateral locked for this position | Formula: `Position Value / Leverage` | Unit: USDT
- **LiqPrice** (Liquidation Price): Price at which position will be force-closed. 0.0000 = no liquidation risk | Unit: USDT
- **UnrealizedPnL%** (Unrealized PnL Percentage): Floating P&L of current position, not realized until closed | Formula: `(Current Price - Entry Price) / Entry Price × Leverage × 100` | Unit: %

### Market Data
- **Volume** (Volume): Trading volume in this period | Unit: base asset
- **OI** (Open Interest): Total value of open contracts. Increasing OI = capital inflow, decreasing = outflow | Unit: USDT
- **OIChange** (OI Change): Open Interest change. Used to determine real capital flow direction | Unit: USDT & %

## 💹 Open Interest (OI) Change Interpretation

- **OI Up + Price Up**: Strong bullish trend (new longs opening, capital flowing into long positions)
- **OI Up + Price Down**: Strong bearish trend (new shorts opening, capital flowing into short positions)
- **OI Down + Price Up**: Shorts covering (shorts stopped out, potential reversal)
- **OI Down + Price Down**: Longs closing (longs stopped out, potential reversal)


---

# You are a professional Hyperliquid USDC multi-asset trading AI

Your task is to make trading decisions based on the provided market data.

# Hard Constraints (Risk Control)

## CODE ENFORCED (backend validation, cannot be bypassed):
- Max Positions: 5 instruments simultaneously
- Position Value Limit (Altcoin/Stock): max 41 USDT (= equity 41 × 1.0x)
- Position Value Limit (BTC/ETH): max 82 USDT (= equity 41 × 2.0x)
- Max Margin Usage: ≤90%
- Min Position Size: ≥10 USDT

## AI GUIDED (recommended):
- Trading Leverage: Altcoin/Stock max 5x | BTC/ETH max 5x
- Risk-Reward Ratio: ≥1:2.0 (take_profit / stop_loss)
- Min Confidence (DO NOT use AI score from AI500 or rank from OI/Net Flow directly as your confidence score): ≥60 to open position

## Position Sizing Guidance
Calculate `position_size_usd` from your confidence and the Position Value Limits above:
- High confidence (≥85): use 80-100%% of the position value limit
- Medium confidence (70-84): use 50-80%% of the position value limit
- Low confidence (60-69): use 30-50%% of the position value limit
- Example: equity 41 × 2.0x = max 82 USDT
- **DO NOT** just use available_balance as position_size_usd. Use the Position Value Limit!

# ⏱️ Trading Frequency Awareness

- Excellent traders: 7-8 trades/day ≈ 0.3-0.4 trades/hour
- >3 trades/hour = overtrading
- Single position hold time ≥ 30-90 minutes
If you find yourself trading every cycle → standards too low; if closing positions < 30 minutes → too impulsive.

# 🎯 Entry Standards (Strict)

Only open positions when multiple signals resonate. You have:
- 15m price series +  K-line series
- EMA indicators (periods: [10 20 50])
- MACD indicators
- RSI indicators (periods: [7 14])
- ATR indicators (periods: [14])
- Bollinger Bands (BOLL) - Upper/Middle/Lower bands (periods: [20])
- Volume data
- Open Interest (OI) data
- AI500 / OI_Top filter tags (if available)

Feel free to use any effective analysis method, but **confidence ≥ 60** is required to open positions; avoid low-quality behaviors such as single-indicator entries, contradictory signals, sideways chop, or re-entering immediately after a close.

# 📋 Decision Process

1. Check positions → take profit / stop loss?
2. Scan candidates + multi-timeframe → are there strong signals?
3. Write chain of thought first, then output structured JSON
4. Your recorded SL/TP are reference levels, not hard constraints — you may take profit or cut the loss early if the thesis is invalidated.

# Output Format (Strictly Follow)

**Must use XML tags <reasoning> and <decision> to separate chain of thought and decision JSON, avoiding parsing errors**

## Format Requirements

<reasoning>
Your chain of thought analysis...
- Briefly analyze your thinking process
</reasoning>

<decision>
Step 2: JSON decision array

```json
[
  {"symbol": "BTCUSDT", "action": "open_short", "leverage": 5, "position_size_usd": 82, "stop_loss": 97000, "take_profit": 91000, "confidence": 85, "risk_usd": 300},
  {"symbol": "ETHUSDT", "action": "close_long"}
]
```
</decision>

## Field Description

- `action`: open_long | open_short | close_long | close_short | hold | wait
- `confidence`: 0-100 (opening recommended ≥ 60)
- Required when opening: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd
- **Price precision**: use the same significant figures as the prices shown in `## Market Data` (e.g. `76708.91` for BTC, `0.0055680` for a sub-cent coin). Do not round prices to a fixed number of decimals.
- **IMPORTANT**: all numeric values must be calculated numbers, NOT formulas/expressions (e.g. use `27.76`, not `3000 * 0.01`)

# 📌 Personalized Trading Strategy

You are a professional high-risk-tolerance quantitative crypto trading AI assistant, specialized in combining the AI500 composite score with momentum indicators to catch strongly-breaking-out coins.

Your job:
By monitoring the AI500 metric data and analyze each candidate's candles, EMA, MACD, RSI, ATR, and Bollinger Bands in real time with Open Interest (OI) and net inflow/outflow data, screen for coins sitting in an extremely strong zone, and execute an aggressive trend-following strategy in a high-volatility environment only after genuine momentum is confirmed.


Core Trading Logic

AI500 Score Admission (mandatory rule):
Entry red line: only consider opening a position when the AI500 composite score is greater and equal to 65.
Liquidation warning: if a held coin's score falls below 65, treat this as sentiment reversing — the position must be force-closed.

Trend & Entry (highly aggressive mode):
Trend confirmation: price must be above EMA50 on both the 15m and 1m timeframes.
Entry timing: once the score reaches 65, enter immediately on a pullback to EMA20 — do not wait for a deeper retracement (to avoid missing a strong move).
Second add-on: if the score continues rising to 73+ accompanied by a volume surge, add 50% to the existing position.

Position Management (high risk tolerance):
Single-trade risk: a single trade's position must not exceed 15% of total capital as margin (kept aggressive on purpose: at 5x leverage this is still ~75% notional exposure). Never use the exact 100% of Position Value Limit (Follow the confidence "Position Sizing Guidance" instead) for position size, use up to 90%.
Concentrate firepower: hold no more than 5 coins at the same time; focus on the highest-scoring breakout coins.

Take-Profit / Stop-Loss:
Aggressive stop-loss: place the stop below the entry point at 5%, or exit manually if the AI500 score drops sharply.
Staged take-profit: at the first target of take profit at 10%, take profit on 50% of the position; the remaining position switches into wave/swing trailing.
Dynamic trailing: once floating profit exceeds 7%, lock the stop-loss at the 5% profit level — don't get shaken out easily by small fluctuations.

Signal Filters:
Volume: must rank near the top of either the 1h / 30m / 15m Inflow leaderboard (make sure it is inflow, not outflow), the 1h / 30m / 15m Open Interest (OI) leaderboard (make sure it is increase OI, not decrease OI) or the current minute's volume ratio > 2.0.


Decision Principles

Score first: even if the technicals are perfect, never open a position when the AI500 score is below 65.
Dare to go big: when multiple indicators resonate and the score is 73+, stay aggressively on the offensive.
Correct fast: the moment a held coin's score falls below 65, close immediately — don't get attached to the fundamentals.
Every action instruction must clearly state: [current AI500 score], [entry/adjustment rationale], and [expected volatility range].

Note: the above personalized strategy supplements the basic rules and may not violate the core risk controls.

--- User Prompt ---
Time: 2026-09-20 08:28:10 UTC | Period: #2 | Runtime: 15 minutes

Account: Equity 41.22 | Balance 33.25 (80.7%) | PnL +51.34% | Margin 17.2% | Positions 1

## Recent Completed Trades
Recent closes: 3 TP, 1 SL, 0 trailing-SL (profit locked), 10 LLM decisions, 0 manual, 1 unknown

1. WLFIUSDT long | Entry 0.058390 Exit 0.058190 | Loss: -0.06 USDT (-0.34%) | 09-20 06:04 UTC→09-20 07:04 UTC (59m) [LLM close]
2. GUSDT long | Entry 0.014812 Exit 0.014607 | Loss: -0.39 USDT (-1.38%) | 09-20 06:35 UTC→09-20 06:50 UTC (14m) [LLM close]
3. USELESSUSDT long | Entry 0.26007 Exit 0.25695 | Loss: -0.19 USDT (-1.20%) | 09-20 05:49 UTC→09-20 06:35 UTC (45m) [LLM close]
4. BRUSDT long | Entry 1.1354 Exit 1.1178 | Loss: -0.46 USDT (-1.55%) | 09-20 05:19 UTC→09-20 05:49 UTC (29m) [LLM close]
5. BRUSDT long | Entry 1.0953 Exit 1.0509 | Loss: -1.29 USDT (-4.05%) | 09-19 23:34 UTC→09-20 03:48 UTC (4h13m) [SL hit]
6. CRVUSDT long | Entry 0.34350 Exit 0.34280 | Loss: -0.04 USDT (-0.20%) | 09-19 23:49 UTC→09-20 00:19 UTC (29m) [LLM close]
7. CRVUSDT long | Entry 0.34280 Exit 0.34220 | Loss: -0.03 USDT (-0.18%) | 09-19 22:49 UTC→09-19 23:34 UTC (44m) [LLM close]
8. BRUSDT long | Entry 1.0612 Exit 1.0939 | Profit: +0.43 USDT (+3.09%) | 09-19 20:49 UTC→09-19 23:04 UTC (2h14m) [LLM close]
9. ONEUSDT long | Entry 0.0047389 Exit 0.0049728 | Profit: +0.91 USDT (+4.94%) | 09-19 21:49 UTC→09-19 22:45 UTC (56m) [TP hit]
10. ONEUSDT long | Entry 0.0043150 Exit 0.0047659 | Profit: +2.87 USDT (+10.45%) | 09-19 21:19 UTC→09-19 21:34 UTC (14m) [TP hit]
11. BRUSDT long | Entry 1.1446 Exit 1.1883 | Profit: +1.05 USDT (+3.82%) | 09-19 17:19 UTC→09-19 20:04 UTC (2h44m) [LLM close]
12. BTCUSDT long | Entry 81783.00 Exit 81751.10 | Loss: -0.03 USDT (-0.04%) | 09-19 16:19 UTC→09-19 17:04 UTC (44m) [LLM close]
13. BRUSDT long | Entry 1.1343 Exit 1.0837 | Loss: -1.11 USDT (-4.47%) | 09-19 16:49 UTC→09-19 17:01 UTC (11m) [exchange]
14. BRUSDT long | Entry 1.0887 Exit 1.1435 | Profit: +1.53 USDT (+5.03%) | 09-19 15:34 UTC→09-19 16:18 UTC (44m) [TP hit]
15. BTCUSDT long | Entry 81437.70 Exit 81489.80 | Profit: +0.05 USDT (+0.06%) | 09-19 14:19 UTC→09-19 16:04 UTC (1h44m) [LLM close]

## Historical Trading Statistics
Total Trades: 249 | Profit Factor: 1.35 | Sharpe: 0.09 | Win/Loss Ratio: 1.45
Total PnL: +19.71 USDT | Avg Win: +0.63 | Avg Loss: -0.44 | Max Drawdown: 18.7%
Performance: NORMAL - room for optimization

## Current Positions
1. BRUSDT LONG | Entry 1.1138 Current 1.1446 | Qty 31.0000 | Position Value 35.48 USDT | PnL+13.45% | PnL Amount+0.95 USDT | Peak PnL18.67% | Leverage 5x | Margin 7 | Liq Price 0.90579 | SL 1.0527 TP 1.2189 | SL Dist -5.49% TP Dist +9.43% | Opened 09-20 06:50 UTC (Holding 1h 38m)

=== BRUSDT Market Data ===

current_price = 1.1462, current_ema20 = 1.127, current_macd = 0.007, current_rsi7 = 61.946

Additional data for BRUSDT:

Open Interest: Latest: 57513473.00 Average: 57455959.53

=== 1M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
09-20 07:59    1.1418      1.1431      1.1399      1.1401      56546.00    
09-20 08:00    1.1401      1.1420      1.1326      1.1338      128156.00   
09-20 08:01    1.1336      1.1337      1.1264      1.1288      184693.00   
09-20 08:02    1.1290      1.1339      1.1279      1.1324      93604.00    
09-20 08:03    1.1322      1.1378      1.1305      1.1374      139141.00   
09-20 08:04    1.1377      1.1427      1.1364      1.1368      135302.00   
09-20 08:05    1.1367      1.1401      1.1353      1.1375      80657.00    
09-20 08:06    1.1375      1.1391      1.1358      1.1371      53274.00    
09-20 08:07    1.1371      1.1404      1.1331      1.1389      76525.00    
09-20 08:08    1.1392      1.1524      1.1387      1.1510      367812.00   
09-20 08:09    1.1510      1.1562      1.1463      1.1513      301148.00   
09-20 08:10    1.1513      1.1796      1.1451      1.1462      827296.00   
09-20 08:11    1.1454      1.1496      1.1395      1.1432      220801.00   
09-20 08:12    1.1432      1.1475      1.1401      1.1434      146883.00   
09-20 08:13    1.1435      1.1555      1.1430      1.1531      156570.00   
09-20 08:14    1.1529      1.1548      1.1490      1.1534      113747.00   
09-20 08:15    1.1539      1.1562      1.1500      1.1542      149300.00   
09-20 08:16    1.1544      1.1567      1.1525      1.1561      99493.00    
09-20 08:17    1.1557      1.1570      1.1475      1.1500      142284.00   
09-20 08:18    1.1500      1.1515      1.1460      1.1503      117044.00   
09-20 08:19    1.1506      1.1592      1.1506      1.1582      173554.00   
09-20 08:20    1.1582      1.1584      1.1512      1.1532      99833.00    
09-20 08:21    1.1531      1.1535      1.1500      1.1514      91084.00    
09-20 08:22    1.1512      1.1535      1.1501      1.1517      87329.00    
09-20 08:23    1.1517      1.1535      1.1480      1.1491      115692.00   
09-20 08:24    1.1490      1.1522      1.1478      1.1508      55913.00    
09-20 08:25    1.1509      1.1514      1.1482      1.1487      43148.00    
09-20 08:26    1.1487      1.1521      1.1475      1.1520      67259.00    
09-20 08:27    1.1520      1.1526      1.1383      1.1454      197902.00   
09-20 08:28    1.1452      1.1479      1.1439      1.1455      37529.00      <- current

EMA10: [1.1396, 1.1385, 1.1367, 1.1359, 1.1362, 1.1363, 1.1365, 1.1366, 1.1370, 1.1396, 1.1417, 1.1425, 1.1427, 1.1428, 1.1447, 1.1463, 1.1477, 1.1492, 1.1494, 1.1495, 1.1511, 1.1515, 1.1515, 1.1515, 1.1511, 1.1510, 1.1506, 1.1508, 1.1498, 1.1491]
EMA20: [1.1389, 1.1384, 1.1375, 1.1370, 1.1371, 1.1370, 1.1371, 1.1371, 1.1372, 1.1386, 1.1398, 1.1404, 1.1407, 1.1409, 1.1421, 1.1432, 1.1442, 1.1453, 1.1458, 1.1462, 1.1474, 1.1479, 1.1482, 1.1486, 1.1486, 1.1488, 1.1488, 1.1491, 1.1488, 1.1484]
EMA50: [1.1344, 1.1344, 1.1342, 1.1341, 1.1342, 1.1343, 1.1344, 1.1346, 1.1347, 1.1354, 1.1360, 1.1364, 1.1367, 1.1369, 1.1376, 1.1382, 1.1388, 1.1395, 1.1399, 1.1403, 1.1410, 1.1415, 1.1419, 1.1423, 1.1425, 1.1428, 1.1431, 1.1434, 1.1435, 1.1436]
MACD: [0.0011, 0.0006, -0.0002, -0.0006, -0.0004, -0.0004, -0.0003, -0.0002, -0.0000, 0.0011, 0.0020, 0.0022, 0.0022, 0.0021, 0.0028, 0.0034, 0.0038, 0.0043, 0.0041, 0.0039, 0.0044, 0.0043, 0.0041, 0.0038, 0.0034, 0.0032, 0.0028, 0.0027, 0.0021, 0.0016]
RSI7: [54.5590, 31.1290, 22.3398, 37.3369, 52.0784, 50.4082, 52.4540, 51.0649, 57.3790, 78.4183, 78.6819, 61.4677, 53.4107, 53.9470, 70.6003, 70.9914, 72.0547, 74.7410, 55.0214, 55.6096, 69.7181, 56.3711, 52.3150, 52.8769, 46.4636, 50.9691, 45.2216, 54.5457, 39.0233, 39.5109]
RSI14: [54.4554, 42.3635, 35.6661, 42.7481, 50.7242, 49.8218, 50.9035, 50.2442, 53.3633, 67.5501, 67.7768, 59.0478, 54.5743, 54.8549, 64.7432, 65.0070, 65.6849, 67.3774, 57.6001, 57.8834, 65.4714, 58.2541, 55.9485, 56.2136, 52.7793, 54.7142, 51.7576, 55.7708, 47.2856, 47.4945]
ATR14: 0.0069150
BOLL(20) Upper: [1.1448, 1.1446, 1.1454, 1.1452, 1.1449, 1.1438, 1.1436, 1.1430, 1.1432, 1.1464, 1.1489, 1.1498, 1.1502, 1.1506, 1.1528, 1.1547, 1.1565, 1.1585, 1.1592, 1.1599, 1.1619, 1.1625, 1.1619, 1.1614, 1.1612, 1.1608, 1.1601, 1.1592, 1.1582, 1.1582]
BOLL(20) Middle: [1.1393, 1.1389, 1.1382, 1.1378, 1.1376, 1.1372, 1.1371, 1.1369, 1.1371, 1.1379, 1.1387, 1.1392, 1.1394, 1.1397, 1.1405, 1.1413, 1.1421, 1.1429, 1.1433, 1.1437, 1.1447, 1.1456, 1.1468, 1.1477, 1.1483, 1.1490, 1.1496, 1.1503, 1.1506, 1.1504]
BOLL(20) Lower: [1.1338, 1.1331, 1.1311, 1.1304, 1.1303, 1.1306, 1.1306, 1.1307, 1.1311, 1.1295, 1.1285, 1.1285, 1.1286, 1.1287, 1.1282, 1.1279, 1.1278, 1.1273, 1.1274, 1.1276, 1.1274, 1.1288, 1.1316, 1.1340, 1.1354, 1.1372, 1.1390, 1.1414, 1.1431, 1.1425]

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
09-20 01:00    1.1328      1.1350      1.0888      1.0929      1939995.00  
09-20 01:15    1.0929      1.1215      1.0926      1.1026      1695178.00  
09-20 01:30    1.1027      1.1136      1.0985      1.1095      922279.00   
09-20 01:45    1.1095      1.1218      1.1056      1.1187      965019.00   
09-20 02:00    1.1187      1.1632      1.1168      1.1205      2716613.00  
09-20 02:15    1.1203      1.1217      1.0956      1.1086      1827249.00  
09-20 02:30    1.1084      1.1105      1.0909      1.0966      965774.00   
09-20 02:45    1.0971      1.1234      1.0957      1.1125      1392982.00  
09-20 03:00    1.1125      1.1332      1.1125      1.1277      1530256.00  
09-20 03:15    1.1277      1.1460      1.1164      1.1427      1469033.00  
09-20 03:30    1.1430      1.1557      1.1358      1.1547      1088799.00  
09-20 03:45    1.1547      1.1577      1.0432      1.1141      5311232.00  
09-20 04:00    1.1141      1.1300      1.0861      1.1112      1567942.00  
09-20 04:15    1.1111      1.1318      1.1090      1.1293      1096137.00  
09-20 04:30    1.1292      1.1319      1.1065      1.1111      1016004.00  
09-20 04:45    1.1109      1.1215      1.0970      1.1202      1248378.00  
09-20 05:00    1.1203      1.1327      1.1139      1.1251      1023011.00  
09-20 05:15    1.1251      1.1395      1.1226      1.1348      985315.00   
09-20 05:30    1.1348      1.1379      1.1106      1.1164      979429.00   
09-20 05:45    1.1163      1.1231      1.1063      1.1207      808186.00   
09-20 06:00    1.1207      1.1256      1.0938      1.1000      1017277.00  
09-20 06:15    1.1000      1.1085      1.0870      1.0915      1098930.00  
09-20 06:30    1.0915      1.1085      1.0810      1.1011      1466703.00  
09-20 06:45    1.1015      1.1237      1.1002      1.1174      1159110.00  
09-20 07:00    1.1174      1.1313      1.1090      1.1289      1110047.00  
09-20 07:15    1.1290      1.1470      1.1245      1.1382      1492126.00  
09-20 07:30    1.1382      1.1480      1.1332      1.1445      1454955.00  
09-20 07:45    1.1445      1.1448      1.1284      1.1401      1267587.00  
09-20 08:00    1.1401      1.1796      1.1264      1.1534      3025609.00  
09-20 08:15    1.1539      1.1592      1.1383      1.1462      1476700.00    <- current

EMA10: [1.1095, 1.1083, 1.1085, 1.1103, 1.1122, 1.1115, 1.1088, 1.1095, 1.1128, 1.1182, 1.1249, 1.1229, 1.1208, 1.1223, 1.1203, 1.1203, 1.1211, 1.1236, 1.1223, 1.1220, 1.1180, 1.1132, 1.1110, 1.1122, 1.1152, 1.1194, 1.1240, 1.1269, 1.1317, 1.1344]
EMA20: [1.1117, 1.1109, 1.1107, 1.1115, 1.1124, 1.1120, 1.1105, 1.1107, 1.1123, 1.1152, 1.1190, 1.1185, 1.1178, 1.1189, 1.1182, 1.1184, 1.1190, 1.1205, 1.1201, 1.1202, 1.1183, 1.1157, 1.1143, 1.1146, 1.1160, 1.1181, 1.1206, 1.1225, 1.1254, 1.1274]
EMA50: [1.0891, 1.0896, 1.0904, 1.0915, 1.0927, 1.0933, 1.0934, 1.0942, 1.0955, 1.0973, 1.0996, 1.1002, 1.1006, 1.1017, 1.1021, 1.1028, 1.1037, 1.1049, 1.1053, 1.1059, 1.1057, 1.1052, 1.1050, 1.1055, 1.1064, 1.1076, 1.1091, 1.1103, 1.1120, 1.1133]
MACD: [-0.0012, -0.0017, -0.0015, -0.0006, 0.0003, -0.0000, -0.0012, -0.0008, 0.0007, 0.0030, 0.0058, 0.0047, 0.0035, 0.0040, 0.0029, 0.0027, 0.0029, 0.0038, 0.0030, 0.0027, 0.0008, -0.0014, -0.0023, -0.0017, -0.0004, 0.0015, 0.0034, 0.0045, 0.0064, 0.0073]
RSI7: [40.5708, 46.3238, 50.2880, 55.4105, 56.4396, 47.9051, 40.6854, 51.9138, 60.3149, 66.9461, 71.3805, 46.5732, 45.2491, 54.5595, 45.4982, 50.3337, 52.9627, 58.0590, 46.8353, 49.4926, 38.6083, 34.9466, 42.2026, 52.6478, 58.7734, 63.2476, 66.1964, 62.1821, 68.8325, 61.9464]
RSI14: [45.8437, 48.2126, 49.8796, 52.0939, 52.5376, 49.2847, 46.1869, 50.6255, 54.4994, 57.9930, 60.5834, 49.4152, 48.7172, 53.1222, 48.6084, 50.8675, 52.0921, 54.4954, 49.4325, 50.5858, 45.2165, 43.1998, 46.1352, 50.7788, 53.8013, 56.1409, 57.7293, 56.2172, 59.6498, 57.0404]
ATR14: 0.028650
BOLL(20) Upper: [1.1314, 1.1314, 1.1315, 1.1325, 1.1310, 1.1313, 1.1304, 1.1310, 1.1332, 1.1391, 1.1474, 1.1476, 1.1472, 1.1482, 1.1466, 1.1467, 1.1473, 1.1490, 1.1486, 1.1473, 1.1466, 1.1477, 1.1480, 1.1479, 1.1487, 1.1510, 1.1533, 1.1553, 1.1592, 1.1597]
BOLL(20) Middle: [1.1020, 1.1028, 1.1040, 1.1045, 1.1070, 1.1073, 1.1063, 1.1070, 1.1077, 1.1100, 1.1125, 1.1129, 1.1138, 1.1157, 1.1169, 1.1179, 1.1187, 1.1195, 1.1191, 1.1185, 1.1189, 1.1183, 1.1179, 1.1178, 1.1182, 1.1197, 1.1221, 1.1235, 1.1248, 1.1250]
BOLL(20) Lower: [1.0726, 1.0742, 1.0766, 1.0764, 1.0829, 1.0833, 1.0822, 1.0830, 1.0822, 1.0809, 1.0776, 1.0781, 1.0805, 1.0832, 1.0872, 1.0892, 1.0900, 1.0899, 1.0896, 1.0897, 1.0911, 1.0889, 1.0878, 1.0877, 1.0878, 1.0884, 1.0909, 1.0917, 1.0904, 1.0902]

=== BRUSDT AI500 Signal ===
AI score 86.2/100 | peak score 93 | current price 1.1462 | start price 0.29625 | change +286.5% since starting alert

=== BRUSDT Open Interest ===
[15m · Increase] rank #9 | OI change +0.2% ($531.7K) | price +0.6% | OI $57.4M | long net -$6.8M / short net $30.0M
[30m · Increase] rank #13 | OI change +0.0% ($658.7K) | price +1.0% | OI $57.5M | long net -$6.7M / short net $30.0M
[1h · Increase] rank #6 | OI change +0.4% ($1.3M) | price +1.5% | OI $57.3M | long net -$6.8M / short net $29.9M
[24h · Increase] rank #4 | OI change -3.4% ($9.6M) | price +21.2% | OI $59.6M | long net -$5.6M / short net $30.9M

=== BRUSDT Net Flow ===
[15m · inflow] rank #14 | net flow $126.8K | price 1.1450
[30m · inflow] rank #10 | net flow $924.3K | price 1.1453
[1h · inflow] rank #8 | net flow $1.2M | price 1.1453
[24h · inflow] rank #2 | net flow $13.2M | price 1.1449

=== BRUSDT Price Change ===
[15m] price change +0.6% | spot -$10.6K / future $126.6K | OI delta $531.7K
[30m] price change +1.0% | spot $34.7K / future $924.3K | OI delta $658.7K
[1h] price change +1.5% | spot $46.7K / future $1.2M | OI delta $1.3M
[24h] price change +21.2% | spot $796.3K / future $13.2M | OI delta $9.6M

=== BRUSDT AltFins ===
[15m] Short Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      Medium Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      Long Term Trend: Strongly Bullish (10/10), changed from Bullish to Strongly Bullish
      MACD Signal: Bullish crossover, 5 bars ago (~75 min ago)
      MACD Histogram: Bullish


## Candidate Coins (3 coins)

### 1. USELESSUSDT (AI500)

=== USELESSUSDT Market Data ===

current_price = 0.26187, current_ema20 = 0.258, current_macd = 0.001, current_rsi7 = 65.811

Additional data for USELESSUSDT:

Open Interest: Latest: 82294163.00 Average: 82211868.84

=== 1M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
09-20 07:59    0.26073     0.26109     0.26041     0.26080     53023.00    
09-20 08:00    0.26081     0.26168     0.26081     0.26123     82877.00    
09-20 08:01    0.26124     0.26147     0.26014     0.26045     116059.00   
09-20 08:02    0.26040     0.26108     0.25973     0.25980     94628.00    
09-20 08:03    0.25980     0.26113     0.25968     0.26101     110888.00   
09-20 08:04    0.26103     0.26263     0.26102     0.26170     246684.00   
09-20 08:05    0.26171     0.26171     0.26081     0.26104     53764.00    
09-20 08:06    0.26101     0.26133     0.26054     0.26125     51739.00    
09-20 08:07    0.26123     0.26196     0.26117     0.26122     57099.00    
09-20 08:08    0.26123     0.26136     0.26038     0.26053     62054.00    
09-20 08:09    0.26054     0.26158     0.26054     0.26122     89740.00    
09-20 08:10    0.26121     0.26181     0.26087     0.26161     66614.00    
09-20 08:11    0.26160     0.26160     0.26071     0.26109     34859.00    
09-20 08:12    0.26108     0.26127     0.26062     0.26102     37561.00    
09-20 08:13    0.26103     0.26106     0.26048     0.26056     27143.00    
09-20 08:14    0.26056     0.26163     0.26039     0.26119     141592.00   
09-20 08:15    0.26118     0.26206     0.26097     0.26181     141640.00   
09-20 08:16    0.26182     0.26193     0.26129     0.26129     75627.00    
09-20 08:17    0.26129     0.26313     0.26118     0.26261     121688.00   
09-20 08:18    0.26262     0.26306     0.26204     0.26271     88907.00    
09-20 08:19    0.26270     0.26271     0.26138     0.26138     79996.00    
09-20 08:20    0.26137     0.26196     0.26108     0.26150     98078.00    
09-20 08:21    0.26149     0.26194     0.26129     0.26175     51046.00    
09-20 08:22    0.26174     0.26179     0.26153     0.26158     36050.00    
09-20 08:23    0.26157     0.26233     0.26146     0.26180     39890.00    
09-20 08:24    0.26181     0.26213     0.26173     0.26173     22338.00    
09-20 08:25    0.26173     0.26212     0.26173     0.26191     21682.00    
09-20 08:26    0.26189     0.26191     0.26163     0.26172     24669.00    
09-20 08:27    0.26171     0.26247     0.26165     0.26184     59085.00    
09-20 08:28    0.26183     0.26190     0.26176     0.26190     3075.00       <- current

EMA10: [0.26081, 0.26088, 0.26080, 0.26062, 0.26069, 0.26088, 0.26091, 0.26097, 0.26101, 0.26093, 0.26098, 0.26109, 0.26109, 0.26108, 0.26099, 0.26102, 0.26117, 0.26119, 0.26145, 0.26168, 0.26162, 0.26160, 0.26163, 0.26162, 0.26165, 0.26167, 0.26171, 0.26171, 0.26174, 0.26177]
EMA20: [0.26085, 0.26088, 0.26084, 0.26074, 0.26077, 0.26086, 0.26088, 0.26091, 0.26094, 0.26090, 0.26093, 0.26100, 0.26101, 0.26101, 0.26096, 0.26099, 0.26106, 0.26109, 0.26123, 0.26137, 0.26137, 0.26138, 0.26142, 0.26143, 0.26147, 0.26149, 0.26153, 0.26155, 0.26158, 0.26161]
EMA50: [0.25990, 0.25995, 0.25997, 0.25997, 0.26001, 0.26007, 0.26011, 0.26016, 0.26020, 0.26021, 0.26025, 0.26030, 0.26034, 0.26036, 0.26037, 0.26040, 0.26046, 0.26049, 0.26057, 0.26066, 0.26069, 0.26072, 0.26076, 0.26079, 0.26083, 0.26086, 0.26091, 0.26094, 0.26097, 0.26101]
MACD: [0.0001, 0.0002, 0.0001, 0.0000, 0.0000, 0.0001, 0.0001, 0.0001, 0.0002, 0.0001, 0.0001, 0.0002, 0.0002, 0.0001, 0.0001, 0.0001, 0.0002, 0.0002, 0.0003, 0.0003, 0.0003, 0.0003, 0.0003, 0.0003, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002]
RSI7: [48.2675, 57.7678, 41.6002, 32.7021, 54.0484, 62.0556, 51.9530, 54.6912, 54.1766, 43.2558, 54.0598, 59.1842, 50.4332, 49.2886, 41.9838, 53.0919, 61.5455, 52.3195, 66.9791, 67.8527, 48.1055, 49.6481, 53.0407, 50.3493, 53.8824, 52.4957, 55.9004, 51.3667, 54.1092, 55.5706]
RSI14: [51.1512, 54.7467, 47.8645, 43.0120, 52.6378, 57.0890, 52.0500, 53.4577, 53.2173, 47.8846, 52.9608, 55.5936, 51.4581, 50.9090, 47.3350, 52.2765, 56.5931, 52.3189, 60.4787, 61.0229, 50.9703, 51.7427, 53.3902, 52.0880, 53.6631, 53.0653, 54.4698, 52.6779, 53.7136, 54.2527]
ATR14: 0.00078054
BOLL(20) Upper: [0.26385, 0.26385, 0.26357, 0.26349, 0.26343, 0.26337, 0.26284, 0.26253, 0.26215, 0.26172, 0.26169, 0.26178, 0.26180, 0.26181, 0.26177, 0.26178, 0.26190, 0.26193, 0.26225, 0.26253, 0.26254, 0.26256, 0.26259, 0.26248, 0.26252, 0.26252, 0.26256, 0.26259, 0.26262, 0.26259]
BOLL(20) Middle: [0.26152, 0.26154, 0.26139, 0.26126, 0.26122, 0.26119, 0.26105, 0.26097, 0.26088, 0.26077, 0.26076, 0.26079, 0.26080, 0.26084, 0.26086, 0.26092, 0.26098, 0.26100, 0.26111, 0.26121, 0.26124, 0.26125, 0.26131, 0.26140, 0.26144, 0.26144, 0.26149, 0.26151, 0.26154, 0.26161]
BOLL(20) Lower: [0.25920, 0.25924, 0.25922, 0.25904, 0.25901, 0.25902, 0.25926, 0.25940, 0.25961, 0.25982, 0.25983, 0.25979, 0.25979, 0.25987, 0.25995, 0.26006, 0.26005, 0.26008, 0.25996, 0.25989, 0.25993, 0.25994, 0.26004, 0.26033, 0.26037, 0.26037, 0.26041, 0.26044, 0.26047, 0.26063]

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
09-20 01:00    0.25801     0.25840     0.25303     0.25553     2830302.00  
09-20 01:15    0.25552     0.25765     0.25475     0.25519     1154909.00  
09-20 01:30    0.25523     0.25733     0.25472     0.25725     1006657.00  
09-20 01:45    0.25725     0.25768     0.25366     0.25479     1648154.00  
09-20 02:00    0.25480     0.25982     0.25478     0.25820     1690095.00  
09-20 02:15    0.25811     0.25836     0.25507     0.25553     1480797.00  
09-20 02:30    0.25553     0.25633     0.24743     0.24900     6067123.00  
09-20 02:45    0.24901     0.25147     0.24540     0.24665     6364364.00  
09-20 03:00    0.24665     0.25148     0.24582     0.24911     3621344.00  
09-20 03:15    0.24912     0.25333     0.24783     0.25139     2095422.00  
09-20 03:30    0.25138     0.25170     0.24920     0.25063     1195016.00  
09-20 03:45    0.25056     0.25223     0.25020     0.25094     851052.00   
09-20 04:00    0.25095     0.25280     0.25069     0.25241     1012089.00  
09-20 04:15    0.25243     0.25530     0.25090     0.25493     1521188.00  
09-20 04:30    0.25488     0.25592     0.25309     0.25348     1063608.00  
09-20 04:45    0.25345     0.25736     0.25236     0.25632     1409974.00  
09-20 05:00    0.25632     0.25956     0.25602     0.25864     1661831.00  
09-20 05:15    0.25856     0.25931     0.25639     0.25908     1752186.00  
09-20 05:30    0.25907     0.26040     0.25745     0.25879     1317689.00  
09-20 05:45    0.25878     0.26190     0.25868     0.26102     1681913.00  
09-20 06:00    0.26101     0.26141     0.25894     0.25947     1299015.00  
09-20 06:15    0.25948     0.26107     0.25748     0.25864     1174071.00  
09-20 06:30    0.25862     0.25878     0.25430     0.25492     1831398.00  
09-20 06:45    0.25489     0.25626     0.25395     0.25484     971693.00   
09-20 07:00    0.25484     0.25591     0.25347     0.25535     1000346.00  
09-20 07:15    0.25533     0.26135     0.25497     0.25976     2209744.00  
09-20 07:30    0.25971     0.26452     0.25877     0.26222     2439477.00  
09-20 07:45    0.26219     0.26443     0.25957     0.26080     1817261.00  
09-20 08:00    0.26081     0.26263     0.25968     0.26119     1273301.00  
09-20 08:15    0.26118     0.26313     0.26097     0.26187     863612.00     <- current

EMA10: [0.25703, 0.25670, 0.25680, 0.25643, 0.25676, 0.25653, 0.25516, 0.25362, 0.25280, 0.25254, 0.25219, 0.25197, 0.25205, 0.25257, 0.25274, 0.25339, 0.25434, 0.25520, 0.25586, 0.25679, 0.25728, 0.25753, 0.25705, 0.25665, 0.25641, 0.25702, 0.25797, 0.25848, 0.25898, 0.25950]
EMA20: [0.25806, 0.25779, 0.25774, 0.25746, 0.25753, 0.25734, 0.25654, 0.25560, 0.25498, 0.25464, 0.25426, 0.25394, 0.25380, 0.25391, 0.25386, 0.25410, 0.25453, 0.25496, 0.25533, 0.25587, 0.25621, 0.25644, 0.25630, 0.25616, 0.25608, 0.25643, 0.25698, 0.25735, 0.25771, 0.25811]
EMA50: [0.26272, 0.26243, 0.26222, 0.26193, 0.26178, 0.26154, 0.26105, 0.26048, 0.26004, 0.25970, 0.25934, 0.25901, 0.25875, 0.25860, 0.25840, 0.25832, 0.25833, 0.25836, 0.25838, 0.25848, 0.25852, 0.25853, 0.25839, 0.25825, 0.25813, 0.25820, 0.25835, 0.25845, 0.25856, 0.25869]
MACD: [-0.0018, -0.0018, -0.0016, -0.0017, -0.0014, -0.0014, -0.0019, -0.0025, -0.0027, -0.0027, -0.0027, -0.0026, -0.0024, -0.0020, -0.0018, -0.0014, -0.0009, -0.0004, -0.0001, 0.0003, 0.0006, 0.0007, 0.0004, 0.0002, 0.0001, 0.0004, 0.0008, 0.0010, 0.0012, 0.0013]
RSI7: [42.4485, 41.2326, 51.1277, 41.4129, 55.1842, 45.4295, 30.1984, 26.4720, 36.1015, 44.0278, 42.0017, 43.2446, 49.2602, 58.1350, 52.0263, 61.3149, 67.3413, 68.4295, 66.7201, 72.8130, 63.4004, 58.6628, 42.1812, 41.8860, 44.7617, 63.1550, 69.7173, 62.2507, 63.5031, 65.8106]
RSI14: [42.8294, 42.2527, 46.9171, 42.5021, 49.5843, 44.9189, 35.9980, 33.4253, 38.3892, 42.6568, 41.6219, 42.2375, 45.1891, 49.9140, 47.3829, 52.4670, 56.1909, 56.8809, 56.2521, 59.9212, 56.3816, 54.5242, 47.0440, 46.8950, 48.0251, 56.6218, 60.5423, 57.3217, 57.9828, 59.1704]
ATR14: 0.0037851
BOLL(20) Upper: [0.26144, 0.26145, 0.26137, 0.26139, 0.26091, 0.26091, 0.26204, 0.26306, 0.26334, 0.26303, 0.26242, 0.26152, 0.26095, 0.26087, 0.26066, 0.26066, 0.26087, 0.26102, 0.26149, 0.26206, 0.26256, 0.26292, 0.26273, 0.26273, 0.26243, 0.26296, 0.26378, 0.26376, 0.26394, 0.26436]
BOLL(20) Middle: [0.25706, 0.25704, 0.25699, 0.25697, 0.25732, 0.25732, 0.25694, 0.25634, 0.25594, 0.25554, 0.25504, 0.25452, 0.25420, 0.25415, 0.25401, 0.25401, 0.25407, 0.25411, 0.25429, 0.25444, 0.25464, 0.25481, 0.25470, 0.25470, 0.25456, 0.25477, 0.25543, 0.25614, 0.25674, 0.25726]
BOLL(20) Lower: [0.25269, 0.25262, 0.25261, 0.25256, 0.25373, 0.25374, 0.25184, 0.24962, 0.24853, 0.24805, 0.24765, 0.24752, 0.24744, 0.24743, 0.24736, 0.24735, 0.24727, 0.24720, 0.24710, 0.24683, 0.24672, 0.24671, 0.24667, 0.24667, 0.24668, 0.24658, 0.24708, 0.24851, 0.24954, 0.25017]

=== USELESSUSDT AI500 Signal ===
AI score 70.5/100 | peak score 71 | current price 0.26187 | start price 0.25600 | change +2.3% since starting alert

=== USELESSUSDT Open Interest ===
[15m · Increase] rank #24 | OI change -0.0% ($69.9K) | price +0.3% | OI $82.3M | long net -$610.7M / short net $666.7M
[1h · Increase] rank #33 | OI change -0.0% ($134.3K) | price +0.7% | OI $82.3M | long net -$610.8M / short net $666.8M
[24h · Decrease] rank #31 | OI change -3.7% (-$2.0M) | price -5.0% | OI $85.5M | long net -$606.7M / short net $665.9M

=== USELESSUSDT Net Flow ===
[15m · inflow] rank #28 | net flow $35.3K | price 0.26186
[1h · inflow] rank #23 | net flow $180.5K | price 0.26187
[24h · outflow] rank #47 | net flow -$2.9M | price 0.26187

=== USELESSUSDT Price Change ===
[15m] price change +0.3% | spot $0.00 / future $35.3K | OI delta $69.9K
[1h] price change +0.7% | spot $0.00 / future $180.5K | OI delta $134.3K

=== USELESSUSDT AltFins ===
[15m] Short Term Trend: Bullish (7/10), changed from Bearish to Bullish
      Medium Term Trend: Neutral (6/10), changed from Bearish to Neutral
      Long Term Trend: Strongly Bearish (0/10), changed from Bearish to Strongly Bearish
      MACD Signal: Bullish crossover, 17 bars ago (~255 min ago)
      MACD Histogram: Bullish

=== USELESSUSDT Vergex Signal Lab ===
- Band: 15
- Bias: bullish
- Confidence: High · full cost/liquidation coverage (note: liq map is public, reflexivity)
- Composite Z: 0

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | weak | -1 | book-wide trapped cost: support below (underwater shorts) $3.32M vs overhead above (underwater longs) $2.76M · longs P&L $134.2K / shorts P&L $-160.9K · trapped shorts below outweigh trapped longs above → net support...<truncated> |
| I Cost & Positioning | Value-area position | bearish | medium | -1 | price $0.26206 · POC $0.27582 (+5.3%) · value area $0.24524~$0.28805 · in the lower value area (leaning bearish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -7.2% vs weighted avg resistance +6.2% from price · trapped support and resistance sit at a similar cost-weighted distance |
| II Liquidation | Liquidation-fuel imbalance (LFA) | neutral | flat | -1 | within ±15%: short liquidations above (squeeze fuel) $19.7K vs long liquidations below (flush fuel) $18.1K |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $0.30028 (+14.6%) · 39% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 0.0% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bullish | weak | -1 | within ±15%: support below (trapped shorts cover) $3.21M @ $0.24524 vs resistance above (trapped longs break even) $2.69M @ $0.27582 |

=== USELESSUSDT Vergex Liquidation Heatmap ===
- Bin step: 0.0031

- Total cost: long $5.56M / short $5.56M
- Total liquidation: long $2.18M / short $379.13K
| Price zone | Long cost | Short cost | Long liq | Short liq | Main cluster |
| --- | ---: | ---: | ---: | ---: | --- |
| 0.2743-0.2773 | $1.09M | $592.44K | $0.00 | $0.00 | long cost |
| 0.2437-0.2468 | $48.44K | $827.16K | $0.00 | $0.00 | short cost |
| 0.1979-0.2009 | $41.35K | $44.55 | $601.92K | $0.00 | long liquidation |
| 0.2621-0.2651 | $51.93K | $525.10K | $0.00 | $0.00 | short cost |
| 0.259-0.2621 | $519.20K | $304.92K | $0.00 | $0.00 | long cost |
| 0.2712-0.2743 | $456.95K | $330.36K | $0.00 | $0.00 | long cost |
| 0.2468-0.2498 | $49.00K | $387.74K | $0.00 | $0.00 | short cost |
| 0.2804-0.2835 | $370.78K | $65.77K | $0.00 | $0.00 | long cost |
| 0.2529-0.2559 | $189.72K | $352.39K | $0.00 | $0.00 | short cost |
| 0.2498-0.2529 | $331.19K | $157.55K | $0.00 | $0.00 | long cost |
- Additional heatmap bins omitted: 93


### 2. ZECUSDT (AI500)

=== ZECUSDT Market Data ===

current_price = 1442.42, current_ema20 = 1451.481, current_macd = -5.041, current_rsi7 = 32.029

Additional data for ZECUSDT:

Open Interest: Latest: 479721.72 Average: 479242.00

=== 1M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
09-20 07:59    1445.83     1446.60     1445.22     1445.61     437.21      
09-20 08:00    1445.62     1447.33     1444.89     1445.25     580.05      
09-20 08:01    1445.26     1447.00     1445.25     1445.94     438.63      
09-20 08:02    1446.01     1447.18     1445.16     1447.12     397.43      
09-20 08:03    1447.13     1449.60     1447.13     1449.01     1096.17     
09-20 08:04    1449.02     1451.61     1448.01     1448.12     928.59      
09-20 08:05    1448.13     1448.13     1446.21     1446.39     521.89      
09-20 08:06    1446.40     1446.59     1445.22     1445.22     269.12      
09-20 08:07    1445.23     1446.80     1445.00     1445.33     570.37      
09-20 08:08    1445.34     1446.73     1445.33     1446.17     232.69      
09-20 08:09    1446.17     1448.81     1446.13     1448.62     558.62      
09-20 08:10    1448.62     1449.78     1447.69     1447.69     347.72      
09-20 08:11    1447.70     1447.70     1446.87     1446.87     187.51      
09-20 08:12    1446.88     1446.88     1445.79     1446.49     245.60      
09-20 08:13    1446.49     1446.84     1445.78     1446.36     229.69      
09-20 08:14    1446.36     1446.36     1445.39     1445.40     199.01      
09-20 08:15    1445.41     1446.76     1445.40     1445.80     282.36      
09-20 08:16    1445.81     1448.52     1445.31     1445.70     721.49      
09-20 08:17    1445.71     1446.80     1445.28     1445.68     368.45      
09-20 08:18    1445.68     1446.28     1445.00     1445.00     248.80      
09-20 08:19    1445.00     1445.67     1444.00     1445.25     464.65      
09-20 08:20    1445.25     1445.81     1443.16     1443.95     435.15      
09-20 08:21    1443.96     1444.09     1442.42     1442.89     458.79      
09-20 08:22    1442.90     1443.82     1442.90     1443.82     242.81      
09-20 08:23    1443.82     1443.88     1442.13     1442.73     493.79      
09-20 08:24    1442.72     1442.99     1442.01     1442.51     353.52      
09-20 08:25    1442.51     1442.53     1441.72     1442.35     358.46      
09-20 08:26    1442.35     1443.13     1442.00     1442.55     350.71      
09-20 08:27    1442.56     1443.51     1442.49     1443.25     311.24      
09-20 08:28    1443.25     1443.28     1442.19     1442.34     297.98        <- current

EMA10: [1444.75, 1444.84, 1445.04, 1445.42, 1446.07, 1446.44, 1446.43, 1446.21, 1446.05, 1446.07, 1446.54, 1446.75, 1446.77, 1446.72, 1446.65, 1446.43, 1446.31, 1446.20, 1446.11, 1445.90, 1445.79, 1445.45, 1444.99, 1444.77, 1444.40, 1444.06, 1443.75, 1443.53, 1443.48, 1443.27]
EMA20: [1445.97, 1445.90, 1445.90, 1446.02, 1446.30, 1446.48, 1446.47, 1446.35, 1446.25, 1446.24, 1446.47, 1446.59, 1446.61, 1446.60, 1446.58, 1446.47, 1446.40, 1446.34, 1446.27, 1446.15, 1446.07, 1445.86, 1445.58, 1445.41, 1445.16, 1444.91, 1444.66, 1444.46, 1444.35, 1444.15]
EMA50: [1448.32, 1448.20, 1448.11, 1448.07, 1448.10, 1448.11, 1448.04, 1447.93, 1447.83, 1447.76, 1447.79, 1447.79, 1447.75, 1447.70, 1447.65, 1447.56, 1447.49, 1447.42, 1447.36, 1447.26, 1447.18, 1447.06, 1446.89, 1446.77, 1446.62, 1446.45, 1446.29, 1446.15, 1446.03, 1445.89]
MACD: [-1.7370, -1.5838, -1.3908, -1.1295, -0.7612, -0.5349, -0.4896, -0.5418, -0.5678, -0.5146, -0.2717, -0.1524, -0.1227, -0.1283, -0.1416, -0.2270, -0.2594, -0.2898, -0.3119, -0.3799, -0.4090, -0.5307, -0.7047, -0.7587, -0.8794, -0.9814, -1.0629, -1.0988, -1.0585, -1.0874]
RSI7: [54.5852, 51.5049, 56.9386, 64.8062, 73.7637, 64.7147, 50.6296, 43.2093, 44.1078, 51.0128, 65.5112, 57.9197, 51.7508, 48.9331, 47.8923, 40.4754, 44.6428, 43.7495, 43.5461, 36.7678, 40.7253, 29.5177, 23.3934, 36.8126, 29.6990, 28.4064, 27.3948, 30.9794, 42.5594, 33.9265]
RSI14: [45.4437, 44.2102, 47.1701, 51.8725, 58.2782, 54.5933, 48.2119, 44.4295, 44.8674, 48.2231, 56.5331, 53.0524, 50.1220, 48.7775, 48.3001, 44.8124, 46.5445, 46.1545, 46.0713, 43.2205, 44.5783, 39.3136, 35.6197, 40.8692, 37.0555, 36.3188, 35.7620, 37.0610, 41.5184, 37.7732]
ATR14: 1.5313
BOLL(20) Upper: [1451.08, 1450.55, 1450.16, 1449.85, 1449.85, 1449.79, 1449.52, 1449.05, 1448.91, 1448.78, 1449.27, 1449.53, 1449.62, 1449.54, 1449.21, 1449.16, 1448.94, 1448.77, 1448.65, 1448.68, 1448.69, 1448.81, 1449.07, 1449.03, 1448.70, 1448.50, 1448.53, 1448.59, 1448.57, 1448.51]
BOLL(20) Middle: [1445.95, 1445.73, 1445.58, 1445.48, 1445.48, 1445.46, 1445.37, 1445.19, 1445.12, 1445.07, 1445.25, 1445.38, 1445.57, 1445.79, 1446.03, 1446.10, 1446.24, 1446.35, 1446.43, 1446.39, 1446.37, 1446.31, 1446.15, 1445.99, 1445.67, 1445.39, 1445.19, 1445.06, 1444.95, 1444.76]
BOLL(20) Lower: [1440.83, 1440.91, 1440.99, 1441.11, 1441.11, 1441.14, 1441.22, 1441.34, 1441.33, 1441.37, 1441.24, 1441.23, 1441.52, 1442.03, 1442.85, 1443.04, 1443.54, 1443.93, 1444.21, 1444.10, 1444.05, 1443.80, 1443.24, 1442.94, 1442.64, 1442.28, 1441.85, 1441.52, 1441.34, 1441.02]

=== 15M Timeframe (oldest → latest) ===

Time(UTC)      Open      High      Low       Close     Volume
09-20 01:00    1473.67     1479.06     1467.01     1469.65     9245.03     
09-20 01:15    1469.66     1475.98     1463.56     1463.76     14101.94    
09-20 01:30    1463.76     1472.28     1453.15     1471.62     31272.00    
09-20 01:45    1471.62     1475.29     1466.61     1474.19     10092.22    
09-20 02:00    1474.18     1474.55     1467.66     1470.65     6600.41     
09-20 02:15    1470.64     1472.33     1457.74     1461.84     12034.69    
09-20 02:30    1461.83     1464.87     1455.29     1457.10     20028.93    
09-20 02:45    1457.10     1462.16     1450.11     1451.25     33729.48    
09-20 03:00    1451.25     1464.88     1440.65     1459.29     43120.01    
09-20 03:15    1459.29     1472.93     1452.60     1453.33     27159.20    
09-20 03:30    1453.34     1455.86     1451.73     1451.74     9114.02     
09-20 03:45    1451.74     1465.52     1451.00     1456.97     10190.00    
09-20 04:00    1456.98     1458.91     1446.70     1449.41     9405.02     
09-20 04:15    1449.42     1449.94     1445.01     1448.97     6238.26     
09-20 04:30    1448.98     1454.71     1445.28     1450.28     10050.60    
09-20 04:45    1450.29     1456.81     1449.04     1454.16     7489.97     
09-20 05:00    1454.15     1458.84     1450.92     1455.14     7972.08     
09-20 05:15    1455.15     1455.15     1443.20     1448.29     11903.22    
09-20 05:30    1448.28     1452.59     1446.31     1452.59     5663.35     
09-20 05:45    1452.59     1459.99     1450.53     1451.06     12200.20    
09-20 06:00    1451.05     1454.00     1448.51     1450.78     7043.99     
09-20 06:15    1450.78     1453.00     1442.00     1448.66     11604.35    
09-20 06:30    1448.66     1451.52     1434.79     1447.67     21287.12    
09-20 06:45    1447.67     1454.73     1445.03     1448.77     11950.84    
09-20 07:00    1448.77     1455.00     1448.73     1451.67     8441.16     
09-20 07:15    1451.67     1458.25     1449.63     1456.77     9177.14     
09-20 07:30    1456.77     1457.39     1448.27     1448.44     6880.79     
09-20 07:45    1448.44     1449.70     1441.07     1445.61     12184.42    
09-20 08:00    1445.62     1451.61     1444.89     1445.40     6803.09     
09-20 08:15    1445.41     1448.52     1441.72     1442.42     5389.47       <- current

EMA10: [1472.65, 1471.03, 1471.14, 1471.69, 1471.50, 1469.75, 1467.45, 1464.50, 1463.55, 1461.70, 1459.89, 1459.36, 1457.55, 1455.99, 1454.95, 1454.81, 1454.87, 1453.67, 1453.47, 1453.04, 1452.63, 1451.90, 1451.13, 1450.70, 1450.88, 1451.95, 1451.31, 1450.28, 1449.39, 1448.12]
EMA20: [1476.06, 1474.89, 1474.58, 1474.54, 1474.17, 1473.00, 1471.48, 1469.56, 1468.58, 1467.13, 1465.66, 1464.83, 1463.36, 1461.99, 1460.88, 1460.24, 1459.75, 1458.66, 1458.08, 1457.41, 1456.78, 1456.01, 1455.21, 1454.60, 1454.32, 1454.55, 1453.97, 1453.18, 1452.44, 1451.48]
EMA50: [1491.46, 1490.38, 1489.64, 1489.04, 1488.32, 1487.28, 1486.09, 1484.73, 1483.73, 1482.54, 1481.33, 1480.37, 1479.16, 1477.98, 1476.89, 1476.00, 1475.18, 1474.13, 1473.28, 1472.41, 1471.56, 1470.66, 1469.76, 1468.94, 1468.26, 1467.81, 1467.05, 1466.21, 1465.39, 1464.49]
MACD: [-5.8967, -6.2097, -5.7572, -5.1320, -4.8661, -5.3051, -5.9667, -6.8838, -6.8824, -7.2784, -7.6325, -7.4058, -7.7468, -7.9608, -7.9332, -7.5117, -7.0177, -7.0971, -6.7354, -6.4974, -6.2591, -6.1703, -6.1093, -5.9042, -5.4448, -4.6160, -4.5786, -4.7229, -4.7988, -5.0414]
RSI7: [38.7094, 28.6576, 49.1964, 54.2236, 46.7841, 33.4563, 28.3813, 23.2936, 40.4191, 33.8778, 32.2532, 42.7828, 33.8977, 33.4264, 36.4938, 45.2160, 47.3469, 35.9451, 45.5484, 42.8797, 42.3500, 38.1830, 36.2403, 40.1853, 49.7485, 62.1609, 42.2667, 37.5087, 37.1466, 32.0288]
RSI14: [38.7943, 34.2483, 43.7252, 46.4433, 43.3382, 36.7525, 33.7784, 30.4982, 39.2324, 35.6553, 34.7452, 40.1564, 35.5653, 35.3123, 36.7551, 40.9556, 42.0033, 37.0538, 41.6982, 40.5517, 40.3331, 38.6352, 37.8343, 39.3391, 43.2400, 49.4022, 41.4808, 39.1821, 39.0094, 36.5470]
ATR14: 9.4942
BOLL(20) Upper: [1481.71, 1482.54, 1481.63, 1480.97, 1480.95, 1481.25, 1482.48, 1484.65, 1484.30, 1484.62, 1484.78, 1483.70, 1481.92, 1481.17, 1480.42, 1479.50, 1478.41, 1477.56, 1476.01, 1473.73, 1472.04, 1471.30, 1468.67, 1464.37, 1460.06, 1458.81, 1458.16, 1458.42, 1457.22, 1457.52]
BOLL(20) Middle: [1474.35, 1473.85, 1473.42, 1473.17, 1473.21, 1472.40, 1471.50, 1470.55, 1469.68, 1468.60, 1467.44, 1466.43, 1464.77, 1463.45, 1462.33, 1461.51, 1460.73, 1459.64, 1458.69, 1457.56, 1456.62, 1455.87, 1454.67, 1453.40, 1452.45, 1452.19, 1451.76, 1451.48, 1450.79, 1450.24]
BOLL(20) Lower: [1466.99, 1465.16, 1465.20, 1465.37, 1465.48, 1463.55, 1460.53, 1456.46, 1455.07, 1452.58, 1450.11, 1449.15, 1447.62, 1445.72, 1444.24, 1443.51, 1443.05, 1441.71, 1441.38, 1441.40, 1441.20, 1440.44, 1440.66, 1442.42, 1444.84, 1445.58, 1445.36, 1444.54, 1444.35, 1442.96]

=== ZECUSDT AI500 Signal ===
AI score 64.7/100 | peak score 90 | current price 1442.42 | start price 612.52 | change +135.6% since starting alert

=== ZECUSDT Open Interest ===
[15m · Decrease] rank #6 | OI change +0.1% (-$1.6M) | price -0.3% | OI $479.3K | long net -$1.0M / short net $1.4M
[30m · Increase] rank #14 | OI change +0.1% ($629.6K) | price -0.0% | OI $479.2K | long net -$1.0M / short net $1.4M
[1h · Decrease] rank #3 | OI change -0.0% (-$6.4M) | price -0.9% | OI $479.9K | long net -$1.0M / short net $1.4M
[24h · Decrease] rank #2 | OI change -4.3% (-$92.2M) | price -7.8% | OI $501.5K | long net -$1.0M / short net $1.4M

=== ZECUSDT Net Flow ===
[15m · outflow] rank #9 | net flow -$293.2K | price 1443.09
[30m · inflow] rank #43 | net flow $64.7K | price 1443.10
[1h · outflow] rank #9 | net flow -$574.2K | price 1443.10
[24h · outflow] rank #3 | net flow -$106.3M | price 1443.28

=== ZECUSDT Price Change ===
[24h] price change -7.8% | spot -$30.1M / future -$106.3M | OI delta -$92.2M

=== ZECUSDT AltFins ===
[15m] Short Term Trend: Strongly Bearish (0/10), changed from Bearish to Strongly Bearish
      Medium Term Trend: Strongly Bearish (0/10), changed from Bearish to Strongly Bearish
      Long Term Trend: Strongly Bearish (0/10), changed from Bearish to Strongly Bearish
      MACD Signal: Bullish crossover, 14 bars ago (~210 min ago)
      MACD Histogram: Bearish

=== ZECUSDT Vergex Signal Lab ===
- Band: 15
- Bias: conflict
- Confidence: Medium · some positions missing cost/liquidation price
- Composite Z: -0.6

| Family | Signal | Direction | Strength | Percentile | Detail |
| --- | --- | --- | --- | ---: | --- |
| I Cost & Positioning | Trapped-cost overhang (CGO) | bullish | flat | 43.3 | book-wide trapped cost: support below (underwater shorts) $313.50M vs overhead above (underwater longs) $115.65M · longs P&L $123.77M / shorts P&L $-191.08M · trapped shorts below outweigh trapped longs above → net s...<truncated> |
| I Cost & Positioning | Value-area position | bullish | weak | 80 | price $1442 · POC $676.679 (-53.1%) · value area $474.757~$1367 · above the value area (acceptance higher → bullish) |
| I Cost & Positioning | Trapped-pin proximity (reference) | neutral | - | - | reference only (not a directional vote) · weighted avg support -32.0% vs weighted avg resistance +2.9% from price · overhead resistance is nearer on a cost-weighted basis |
| II Liquidation | Liquidation-fuel imbalance (LFA) | bearish | weak | 33.3 | within ±15%: short liquidations above (squeeze fuel) $12.35M vs long liquidations below (flush fuel) $41.17M |
| II Liquidation | Liquidation convexity / magnet | neutral | - | - | Densest liq cluster $1367 (-5.2%) · 28% of band fuel |
| II Liquidation | Cascade vulnerability | neutral | - | - | 1.0% of OI within ±5% of liquidation |
| III Cost Walls | Break-even walls (S/R) | bearish | medium | 6.7 | within ±15%: support below (trapped shorts cover) $73.84M @ $676.679 vs resistance above (trapped longs break even) $115.65M @ $1484 |

=== ZECUSDT Vergex Liquidation Heatmap ===
- Bin step: 16.8268

- Total cost: long $363.69M / short $365.04M
- Total liquidation: long $209.08M / short $151.86M
| Price zone | Long cost | Short cost | Long liq | Short liq | Main cluster |
| --- | ---: | ---: | ---: | ---: | --- |
| 668.2657-685.0925 | $2.80M | $57.21M | $20.91M | $0.00 | short cost |
| 1475.9537-1492.7805 | $39.68M | $4.34M | $0.00 | $0.00 | long cost |
| 1206.7243-1223.5512 | $23.08M | $36.34M | $345.68K | $0.00 | short cost |
| 1442.3-1459.1268 | $30.47M | $5.10M | $0.00 | $0.00 | long cost |
| 1374.9927-1391.8195 | $1.58M | $21.99M | $3.81M | $0.00 | short cost |
| 1189.8975-1206.7243 | $21.89M | $8.38M | $21.19M | $0.00 | long cost |
| 1459.1268-1475.9537 | $20.71M | $9.75M | $0.00 | $29.25K | long cost |
| 1358.1658-1374.9927 | $3.88M | $20.14M | $14.90M | $0.00 | short cost |
| 2266.8148-2283.6417 | $0.00 | $0.00 | $0.00 | $19.97M | short liquidation |
| 1173.0707-1189.8975 | $19.93M | $5.01M | $2.35M | $0.00 | long cost |
- Additional heatmap bins omitted: 110



---

Now please analyze briefly and output the decision JSON.

--- Assistant Response ---
<reasoning>
**Position check — BRUSDT LONG:**
- Entry 1.1138, current 1.1462, PnL +13.45%, Peak +18.67%. AI500 score 86.2 (well above 65 red line). 15m EMA20=1.1274, EMA50=1.1133, price above both → trend intact. 1m EMA20=1.1484, price 1.1462 slightly below 1m EMA20 after the 08:10 spike to 1.1796 and pullback. MACD 1m rolling over (0.007→0.0016), RSI7 1m dropped to 39. This is a normal pullback after a +18% peak.
- Strategy: floating profit >7%, so lock stop at 5% profit level → move SL from 1.0527 to ~1.169? No — 5% profit level = entry × 1.05 = 1.1695, which is ABOVE current price 1.1462. That's not executable. The rule means once floating profit exceeds 7%, lock the stop at the 5% profit level; but price has retraced from peak 18.67% to 13.45%, still above 7%. A stop at 5% profit (1.1695) would be above market — impossible. So instead I keep a protective stop that locks meaningful profit: raise SL to ~1.118 (breakeven+) or tighter. Given the 1m momentum is fading and price is below 1m EMA20, I'll raise SL to 1.1180 (locks ~+0.4% price / ~+2% at 5x) — actually better to protect more. Let me set SL at 1.1200, which locks in ~+2.8% at 5x. Hmm, but the strategy says don't get shaken out by small fluctuations. Price is still above 15m EMA20 (1.1274). Keep SL at 1.1180 (below 15m EMA20 and below the recent consolidation low 1.1264). That locks ~+1.9% at 5x profit. Reasonable.
- Actually the recent 1m low was 1.1264 (08:01). A stop below that at 1.1180 gives room. Keep TP 1.2189.

**Candidates:**
1. **USELESSUSDT** — AI500 70.5 (≥65 ✓). But: 15m EMA50=0.25869, price 0.26187 above it ✓; 1m EMA50=0.26101, price above ✓. However 15m long-term trend Strongly Bearish (0/10), 24h net outflow -$2.9M, OI 24h decreasing. Price is in lower value area (bearish per Vergex). Momentum is weak — MACD flat, RSI7 15m 65.8 but 1m RSI 55.6. Recent trade history shows I already lost on USELESSUSDT long (-1.20%). Volume ranks are mediocre (#23-28 inflow). Not a strong breakout. Skip — insufficient resonance.

2. **ZECUSDT** — AI500 64.7 < 65 → RED LINE, cannot open. Also all trends Strongly Bearish (0/10), 24h outflow -$106M, price -7.8%. Definitely skip.

**Conclusion:** Hold BRUSDT, adjust SL up to lock profit. No new positions meet the strict entry standard (USELESS weak, ZEC below score threshold).
</reasoning>

<decision>
```json
[
  {"symbol": "BRUSDT", "action": "hold", "stop_loss": 1.1180, "take_profit": 1.2189, "confidence": 82}
]
```
</decision>
