// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway;

import com.match.infrastructure.gateway.state.GatewayStateManager;

import java.time.Duration;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

/** Advances zero-activity candle buckets while a market has no executions. */
public final class CandleHeartbeatScheduler implements AutoCloseable {
    private final GatewayStateManager stateManager;
    private final Duration interval;
    private final ScheduledExecutorService scheduler;
    private final AtomicBoolean started = new AtomicBoolean();

    public CandleHeartbeatScheduler(GatewayStateManager stateManager, Duration interval) {
        if (stateManager == null) {
            throw new IllegalArgumentException("stateManager is required");
        }
        if (interval == null || interval.isZero() || interval.isNegative()) {
            throw new IllegalArgumentException("interval must be positive");
        }
        this.stateManager = stateManager;
        this.interval = interval;
        this.scheduler = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread thread = new Thread(r, "spot-candle-heartbeat");
            thread.setDaemon(true);
            return thread;
        });
    }

    public void start() {
        if (started.compareAndSet(false, true)) {
            scheduler.scheduleWithFixedDelay(
                    this::tickSafely, 0, interval.toMillis(), TimeUnit.MILLISECONDS);
        }
    }

    private void tickSafely() {
        try {
            stateManager.broadcastIdleCandleUpdates(System.currentTimeMillis());
        } catch (Throwable ignored) {
            // A heartbeat must never terminate the scheduler or the gateway.
        }
    }

    @Override
    public void close() {
        scheduler.shutdownNow();
    }
}
