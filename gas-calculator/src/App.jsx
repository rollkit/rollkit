import React, { useState } from 'react';

const DEFAULTS = {
  celestiaBlockSizeMB: 8, // 8 MB
  avgTxSize: 250,         // bytes
  avgGasPerTx: 50000,     // gas
};

// Celestia color palette
const COLORS = {
  bgGradient: 'linear-gradient(135deg, #6D4AFF 0%, #00E1FF 100%)',
  cardBg: 'rgba(255,255,255,0.95)',
  accent: '#6D4AFF',
  accent2: '#00E1FF',
  text: '#1a1a1a',
  heading: '#6D4AFF',
  inputBg: '#F3F0FF',
  inputBorder: '#A3F7FF',
  label: '#6D4AFF',
  mathBg: '#F3F0FF',
};

function calculate({ celestiaBlockSizeMB, avgTxSize, avgGasPerTx }) {
  const celestiaBlockSize = Math.floor(Number(celestiaBlockSizeMB) * 1024 * 1024); // MB to bytes
  const numTxs = Math.floor(celestiaBlockSize / avgTxSize);
  const totalGas = numTxs * avgGasPerTx;
  return { celestiaBlockSize, numTxs, totalGas };
}

function formatMGas(gas) {
  return (gas / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 2 });
}

function formatGGas(gas) {
  return (gas / 1_000_000_000).toLocaleString(undefined, { maximumFractionDigits: 3 });
}

export default function App() {
  const [celestiaBlockSizeMB, setCelestiaBlockSizeMB] = useState(DEFAULTS.celestiaBlockSizeMB);
  const [avgTxSize, setAvgTxSize] = useState(DEFAULTS.avgTxSize);
  const [avgGasPerTx, setAvgGasPerTx] = useState(DEFAULTS.avgGasPerTx);

  const { celestiaBlockSize, numTxs, totalGas } = calculate({ celestiaBlockSizeMB, avgTxSize, avgGasPerTx });

  return (
    <div style={{ minHeight: '100vh', background: COLORS.bgGradient, padding: '0', margin: 0 }}>
      <div style={{ maxWidth: 600, margin: '2rem auto', fontFamily: 'Inter, sans-serif', padding: 32, background: COLORS.cardBg, borderRadius: 18, boxShadow: '0 4px 24px #6D4AFF22', color: COLORS.text }}>
        <h1 style={{ color: COLORS.heading, fontWeight: 800, fontSize: '2.2rem', letterSpacing: '-1px', marginBottom: 8 }}>Celestia Rollup Gas Limit Calculator</h1>
        <p style={{ color: COLORS.text, fontSize: '1.1rem', marginBottom: 24 }}>Play with the parameters below to see how the gas limit per block is calculated for an EVM-based rollup using <span style={{ color: COLORS.accent2, fontWeight: 600 }}>Celestia</span> as the DA layer.</p>
        <form style={{ display: 'grid', gap: 20, marginBottom: 32 }} onSubmit={e => e.preventDefault()}>
          <label style={{ color: COLORS.label, fontWeight: 600 }}>
            Celestia Block Size (MB):
            <input
              type="number"
              min={0.01}
              step={0.01}
              value={celestiaBlockSizeMB}
              onChange={e => setCelestiaBlockSizeMB(e.target.value)}
              style={{ marginLeft: 12, width: 120, padding: '8px 10px', borderRadius: 8, border: `1.5px solid ${COLORS.inputBorder}`, background: COLORS.inputBg, fontSize: '1rem', color: COLORS.text }}
            />
          </label>
          <label style={{ color: COLORS.label, fontWeight: 600 }}>
            Average EVM Transaction Size (bytes):
            <input
              type="number"
              min={1}
              value={avgTxSize}
              onChange={e => setAvgTxSize(Number(e.target.value))}
              style={{ marginLeft: 12, width: 120, padding: '8px 10px', borderRadius: 8, border: `1.5px solid ${COLORS.inputBorder}`, background: COLORS.inputBg, fontSize: '1rem', color: COLORS.text }}
            />
          </label>
          <label style={{ color: COLORS.label, fontWeight: 600 }}>
            Average Gas per Transaction:
            <input
              type="number"
              min={1}
              value={avgGasPerTx}
              onChange={e => setAvgGasPerTx(Number(e.target.value))}
              style={{ marginLeft: 12, width: 120, padding: '8px 10px', borderRadius: 8, border: `1.5px solid ${COLORS.inputBorder}`, background: COLORS.inputBg, fontSize: '1rem', color: COLORS.text }}
            />
          </label>
        </form>
        <h2 style={{ color: COLORS.accent, fontWeight: 700, marginBottom: 12 }}>Results</h2>
        <div style={{ background: '#fff', padding: 20, borderRadius: 12, boxShadow: '0 2px 8px #00E1FF22', marginBottom: 16 }}>
          <p><strong style={{ color: COLORS.accent }}>Celestia Block Size (bytes):</strong> {celestiaBlockSize.toLocaleString()}</p>
          <p><strong style={{ color: COLORS.accent }}>Number of EVM Transactions per Block:</strong> {numTxs.toLocaleString()}</p>
          <p><strong style={{ color: COLORS.accent }}>Total Gas Limit per Block:</strong> {totalGas.toLocaleString()} gas</p>
          <p>
            <strong style={{ color: COLORS.accent }}>In Millions of Gas (MGas/block):</strong> {formatMGas(totalGas)} MGas
            {totalGas >= 1_000_000_000 && (
              <><br /><strong style={{ color: COLORS.accent }}>In Billions of Gas (GGas/block):</strong> {formatGGas(totalGas)} GGas</>
            )}
          </p>
          <h3 style={{ color: COLORS.accent2, fontWeight: 700, marginTop: 24 }}>Math & Reasoning</h3>
          <pre style={{ background: COLORS.mathBg, padding: 14, borderRadius: 8, color: COLORS.text, fontSize: '1em', marginBottom: 0 }}>
{`Celestia Block Size (bytes) = Celestia Block Size (MB) * 1024 * 1024
                           = ${celestiaBlockSizeMB} * 1024 * 1024
                           = ${celestiaBlockSize}
NumTxs = Celestia Block Size (bytes) / Avg EVM Tx Size
      = ${celestiaBlockSize} / ${avgTxSize}
      = ${numTxs}

Total Gas Limit = NumTxs * Avg Gas per Tx
               = ${numTxs} * ${avgGasPerTx}
               = ${totalGas}`}
          </pre>
          <p style={{ fontSize: '0.95em', color: '#555', marginTop: 16 }}>
            This tool helps you estimate the EVM gas limit per rollup block based on Celestia DA capacity and your transaction assumptions. Inspired by <a href="https://www.datalenses.zone/chain/celestia/calculator" target="_blank" rel="noopener noreferrer" style={{ color: COLORS.accent2, textDecoration: 'underline' }}>Data Lenses Celestia Calculator</a>.
          </p>
        </div>
      </div>
    </div>
  );
} 