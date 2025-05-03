# Rollup Gas Limit Calculator

This is a simple web tool to help you estimate the EVM gas limit per rollup block based on Celestia DA capacity and your transaction assumptions.

## Features
- Set Celestia block size (in bytes)
- Set average EVM transaction size (in bytes)
- Set average gas per transaction
- See the number of EVM transactions per block and the total gas limit per block
- See the math and reasoning for the calculation

## Usage

1. Install dependencies:
   ```bash
   npm install
   ```
2. Start the development server:
   ```bash
   npm run dev
   ```
3. Open your browser to the local server (usually http://localhost:5173)

## Formula

```
NumTxs = Celestia Block Size / Avg EVM Tx Size
Total Gas Limit = NumTxs * Avg Gas per Tx
```

Inspired by [Data Lenses Celestia Calculator](https://www.datalenses.zone/chain/celestia/calculator). 