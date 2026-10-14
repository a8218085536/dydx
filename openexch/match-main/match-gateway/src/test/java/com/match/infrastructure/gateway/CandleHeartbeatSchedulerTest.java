// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway;

import com.match.infrastructure.gateway.state.GatewayStateManager;
import org.junit.Test;

import java.time.Duration;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.Assert.assertTrue;

public class CandleHeartbeatSchedulerTest {

    @Test
    public void startChecksIdleCandlesInTheBackground() throws Exception {
        AtomicInteger calls = new AtomicInteger();
        GatewayStateManager state = new GatewayStateManager() {
            @Override
            public synchronized int broadcastIdleCandleUpdates(long nowMs) {
                calls.incrementAndGet();
                return 0;
            }
        };

        try (CandleHeartbeatScheduler scheduler =
                     new CandleHeartbeatScheduler(state, Duration.ofMillis(20))) {
            scheduler.start();
            long deadline = System.nanoTime() + Duration.ofSeconds(1).toNanos();
            while (calls.get() == 0 && System.nanoTime() < deadline) {
                Thread.sleep(10);
            }
        }

        assertTrue("background scheduler must check candle boundaries", calls.get() > 0);
    }
}
