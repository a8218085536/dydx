// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.persistence;

/**
 * Rolling 24h ticker figures computed from the 1m candle aggregate.
 * open24h is the close of the last bucket at/before the window start
 * (or the first open inside the window for markets younger than 24h).
 * Fields are 0 when the database has no data for the market.
 */
public record TickerBaseline(double open24h, double high24h, double low24h,
                             double quoteVolume24h, double lastClose, long tradeCount24h,
                             long asOfMs) {

    /** Backward-compatible constructor for callers that do not have trade counts. */
    public TickerBaseline(double open24h, double high24h, double low24h,
                          double quoteVolume24h, double lastClose, long asOfMs) {
        this(open24h, high24h, low24h, quoteVolume24h, lastClose, 0, asOfMs);
    }

    public boolean hasData() {
        return lastClose > 0;
    }
}
