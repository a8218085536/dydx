// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.websocket;

import java.util.List;
import java.util.concurrent.CompletionStage;
import java.util.function.Consumer;
import java.util.stream.IntStream;

/** Public market-data source used by the browser-facing OMS WebSocket. */
public interface MarketDataSource extends AutoCloseable {

    void start(IntStream marketIds, Consumer<String> liveFrameConsumer);

    CompletionStage<List<String>> loadInitialFrames(
            int marketId, String interval, int tradeLimit, int candleLimit);

    @Override
    void close();
}
