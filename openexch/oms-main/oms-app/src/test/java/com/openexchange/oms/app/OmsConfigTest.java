// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * Pins {@link OmsConfig#loadDefaults()} parsing of the Assets Engine client knobs
 * (OMS_AE_HOLD_TIMEOUT_MS / OMS_AE_ACK_TIMEOUT_MS / OMS_AE_CONNECT_TIMEOUT_MS):
 * defaults, explicit overrides, and garbage-input failure. There is no balance-store
 * selection — the AE is the money authority, so nothing to configure.
 *
 * <p>{@code OmsConfig.prop()} checks {@code System.getenv(KEY)} first, falling back to the system
 * property {@code KEY.toLowerCase().replace('_','.')}. Overriding real env vars isn't possible
 * from a plain JUnit test, so these tests drive the system-property fallback instead — every
 * property set here is cleared in {@link #tearDown()} so no test bleeds into another in the same
 * forked JVM.</p>
 */
class OmsConfigTest {

    private static final String HOLD_TIMEOUT_PROP = "oms.ae.hold.timeout.ms";
    private static final String ACK_TIMEOUT_PROP = "oms.ae.ack.timeout.ms";
    private static final String CONNECT_TIMEOUT_PROP = "oms.ae.connect.timeout.ms";
    private static final String MARKET_GATEWAY_URL_PROP = "oms.market.gateway.url";
    private static final String TYCHE_REDIS_URI_PROP = "tyche.redis.uri";
    private static final String TYCHE_REDIS_NAMESPACE_PROP = "tyche.redis.namespace";

    @AfterEach
    void tearDown() {
        System.clearProperty(HOLD_TIMEOUT_PROP);
        System.clearProperty(ACK_TIMEOUT_PROP);
        System.clearProperty(CONNECT_TIMEOUT_PROP);
        System.clearProperty(MARKET_GATEWAY_URL_PROP);
        System.clearProperty(TYCHE_REDIS_URI_PROP);
        System.clearProperty(TYCHE_REDIS_NAMESPACE_PROP);
    }

    @Test
    void defaultsWhenUnset() {
        OmsConfig config = OmsConfig.loadDefaults();
        assertEquals(250L, config.aeHoldTimeoutMs());
        assertEquals(1000L, config.aeAckTimeoutMs());
        assertEquals(30_000L, config.aeConnectTimeoutMs());
        assertEquals("http://127.0.0.1:8081", config.marketGatewayUrl());
        assertEquals("redis://127.0.0.1:6379/0", config.tycheRedisUri());
        assertEquals("", config.tycheRedisNamespace());
    }

    @Test
    void holdTimeoutOverride() {
        System.setProperty(HOLD_TIMEOUT_PROP, "77");
        assertEquals(77L, OmsConfig.loadDefaults().aeHoldTimeoutMs());
    }

    @Test
    void ackTimeoutOverride() {
        System.setProperty(ACK_TIMEOUT_PROP, "4321");
        assertEquals(4321L, OmsConfig.loadDefaults().aeAckTimeoutMs());
    }

    @Test
    void connectTimeoutOverride() {
        System.setProperty(CONNECT_TIMEOUT_PROP, "60000");
        assertEquals(60_000L, OmsConfig.loadDefaults().aeConnectTimeoutMs());
    }

    @Test
    void marketGatewayUrlOverride() {
        System.setProperty(MARKET_GATEWAY_URL_PROP, "http://127.0.0.1:28082");
        assertEquals("http://127.0.0.1:28082", OmsConfig.loadDefaults().marketGatewayUrl());
    }

    @Test
    void tycheRedisOverrides() {
        System.setProperty(TYCHE_REDIS_URI_PROP, "redis://127.0.0.1:6379/2");
        System.setProperty(TYCHE_REDIS_NAMESPACE_PROP, "flydex:tyche:mainnet");
        OmsConfig config = OmsConfig.loadDefaults();
        assertEquals("redis://127.0.0.1:6379/2", config.tycheRedisUri());
        assertEquals("flydex:tyche:mainnet", config.tycheRedisNamespace());
    }

    @Test
    void garbageNumericValueThrows() {
        System.setProperty(HOLD_TIMEOUT_PROP, "not-a-number");
        assertThrows(NumberFormatException.class, OmsConfig::loadDefaults);
    }
}
