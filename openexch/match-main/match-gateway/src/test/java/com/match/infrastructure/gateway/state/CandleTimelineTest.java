// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.state;

import org.junit.Test;

import java.util.List;

import static org.junit.Assert.assertEquals;

public class CandleTimelineTest {

    @Test
    public void fillsMissingBucketsWithPreviousCloseAndZeroActivity() {
        Candle first = candle(1, 60, 100.0, 2.0, 3);
        Candle third = candle(1, 180, 102.0, 4.0, 5);

        List<Candle> result = CandleTimeline.fill(
                List.of(first, third), 1, "1m", 3, 240);

        assertEquals(3, result.size());
        assertEquals(60L, result.get(0).time);
        assertEquals(120L, result.get(1).time);
        assertEquals(100.0, result.get(1).open, 0.0);
        assertEquals(100.0, result.get(1).close, 0.0);
        assertEquals(0.0, result.get(1).volume, 0.0);
        assertEquals(0, result.get(1).tradeCount);
        assertEquals(180L, result.get(2).time);
        assertEquals(102.0, result.get(2).close, 0.0);
    }

    @Test
    public void beforeIsExclusiveAndLimitsTheReturnedTimeline() {
        List<Candle> result = CandleTimeline.fill(List.of(
                candle(1, 60, 100.0, 1.0, 1),
                candle(1, 120, 101.0, 1.0, 1),
                candle(1, 180, 102.0, 1.0, 1)
        ), 1, "1m", 2, 180);

        assertEquals(2, result.size());
        assertEquals(60L, result.get(0).time);
        assertEquals(120L, result.get(1).time);
    }

    private static Candle candle(int marketId, long time, double price,
                                 double volume, int tradeCount) {
        Candle candle = new Candle();
        candle.marketId = marketId;
        candle.time = time;
        candle.open = price;
        candle.high = price;
        candle.low = price;
        candle.close = price;
        candle.volume = volume;
        candle.tradeCount = tradeCount;
        return candle;
    }
}
