// SPDX-License-Identifier: Apache-2.0
import { useState, useCallback } from 'react';
import type { MarketStats, AggregatedTrade, BookLevel } from '../types/market';

/**
 * Hook for managing market ticker stats.
 * Stats are primarily received from the server via TICKER_STATS messages.
 * handleTrades and handleBookUpdate provide fallback updates for last price.
 *
 * `stats` is null until the first TICKER_STATS for the current market lands
 * (and again after resetStats on a market switch) — the ticker rail renders
 * placeholders for null instead of a misleading $0.00.
 */
export function useMarketStats() {
  const [stats, setStats] = useState<MarketStats | null>(null);

  // Fallback: update lastPrice from trades if TICKER_STATS hasn't arrived yet.
  // A null baseline stays null — partial stats would render as fake zeros.
  const handleTrades = useCallback((trades: AggregatedTrade[]) => {
    if (trades.length === 0) return;
    const lastTrade = trades[trades.length - 1];
    setStats(prev => (prev ? { ...prev, lastPrice: lastTrade.price } : prev));
  }, []);

  // An order-book midpoint is an index/reference price, not a last trade.
  // Keep it out of lastPrice so the UI never presents an inferred value as a trade.
  const handleBookUpdate = useCallback((_bids: BookLevel[], _asks: BookLevel[]) => {
    // Intentionally no-op; ticker stats remain authoritative for lastPrice.
  }, []);

  const resetStats = useCallback(() => {
    setStats(null);
  }, []);

  return {
    stats,
    setStats,  // Exposed for direct updates from TICKER_STATS messages
    handleTrades,
    handleBookUpdate,
    resetStats,
  };
}
