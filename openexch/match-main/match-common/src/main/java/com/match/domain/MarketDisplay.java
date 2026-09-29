package com.match.domain;

import java.util.Map;

/**
 * Public spot-market names. Chain-prefixed assets stay internal to settlement
 * and ledger code; market-data responses use the names that users recognize.
 */
public final class MarketDisplay {
    private static final Map<Integer, String> SYMBOLS = Map.of(
            1, "ETH/USDT",
            2, "BNB/USDT",
            3, "BTC/USDT");

    private MarketDisplay() {
    }

    public static String symbol(int marketId) {
        return SYMBOLS.get(marketId);
    }

    public static String symbolOrUnknown(int marketId) {
        String symbol = symbol(marketId);
        return symbol == null ? "UNKNOWN" : symbol;
    }

    public static boolean isSupported(int marketId) {
        return SYMBOLS.containsKey(marketId);
    }
}
