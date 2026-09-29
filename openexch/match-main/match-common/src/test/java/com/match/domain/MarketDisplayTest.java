package com.match.domain;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

public class MarketDisplayTest {

    @Test
    public void exposesNecoMarketsWithFrontendSymbols() {
        assertEquals("ETH/USDT", MarketDisplay.symbol(1));
        assertEquals("BNB/USDT", MarketDisplay.symbol(2));
        assertEquals("BTC/USDT", MarketDisplay.symbol(3));
    }

    @Test
    public void returnsNullForUnknownMarket() {
        assertNull(MarketDisplay.symbol(99));
    }
}
