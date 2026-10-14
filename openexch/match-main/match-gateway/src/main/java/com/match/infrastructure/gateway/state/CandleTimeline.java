// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.state;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/** Builds a continuous candle timeline without inventing trades or volume. */
public final class CandleTimeline {
    private CandleTimeline() {}

    public static List<Candle> fill(List<Candle> actualCandles, int marketId,
                                    String interval, int limit, long beforeExclusiveSec) {
        long intervalSec = intervalSeconds(interval);
        if (intervalSec <= 0 || limit <= 0 || beforeExclusiveSec <= 0 || actualCandles.isEmpty()) {
            return List.of();
        }

        long endBucket = ((beforeExclusiveSec - 1) / intervalSec) * intervalSec;
        long startBucket = endBucket - (long) (limit - 1) * intervalSec;
        List<Candle> sorted = new ArrayList<>(actualCandles);
        sorted.sort(Comparator.comparingLong(c -> c.time));

        Map<Long, Candle> actualByBucket = new HashMap<>();
        Double previousClose = null;
        for (Candle candle : sorted) {
            if (candle.time > endBucket) {
                break;
            }
            if (candle.time < startBucket) {
                previousClose = candle.close;
            } else {
                actualByBucket.put(candle.time, candle);
            }
        }

        List<Candle> result = new ArrayList<>(limit);
        for (long bucket = startBucket; bucket <= endBucket; bucket += intervalSec) {
            Candle actual = actualByBucket.get(bucket);
            if (actual != null) {
                Candle copy = new Candle();
                copy.copyFrom(actual);
                result.add(copy);
                previousClose = actual.close;
            } else if (previousClose != null) {
                result.add(emptyCandle(marketId, bucket, previousClose));
            }
        }
        return result;
    }

    public static long intervalSeconds(String interval) {
        return switch (interval) {
            case "1m" -> 60L;
            case "5m" -> 300L;
            case "15m" -> 900L;
            case "1h" -> 3_600L;
            case "4h" -> 14_400L;
            case "1d" -> 86_400L;
            default -> -1L;
        };
    }

    public static Candle emptyCandle(int marketId, long bucketSec, double close) {
        Candle candle = new Candle();
        candle.marketId = marketId;
        candle.time = bucketSec;
        candle.open = close;
        candle.high = close;
        candle.low = close;
        candle.close = close;
        candle.volume = 0.0;
        candle.tradeCount = 0;
        return candle;
    }
}
