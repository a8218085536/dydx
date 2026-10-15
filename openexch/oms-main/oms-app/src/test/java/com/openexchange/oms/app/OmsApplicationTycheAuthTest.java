// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.zaxxer.hikari.HikariDataSource;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.lang.reflect.Field;
import java.util.concurrent.atomic.AtomicBoolean;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class OmsApplicationTycheAuthTest {

    @AfterEach
    void tearDown() {
        System.clearProperty("oms.auth.mode");
        System.clearProperty("oms.api.keys");
        System.clearProperty("oms.api.keys.file");
        System.clearProperty("tyche.redis.namespace");
    }

    @Test
    void tycheModeRequiresPostgresUserMapping() {
        System.setProperty("oms.auth.mode", "tyche");
        System.setProperty("tyche.redis.namespace", "flydex:tyche:mainnet");

        IllegalStateException error = assertThrows(IllegalStateException.class,
                () -> OmsApplication.buildAuthProvider(OmsConfig.loadDefaults(), null, null));

        assertEquals("OMS_AUTH_MODE=tyche requires PostgreSQL for Tyche-to-OMS user mapping",
                error.getMessage());
    }

    @Test
    void tycheModeRequiresRedisNamespace() {
        System.setProperty("oms.auth.mode", "tyche");

        IllegalArgumentException error;
        try (HikariDataSource dataSource = new HikariDataSource()) {
            error = assertThrows(IllegalArgumentException.class,
                    () -> OmsApplication.buildAuthProvider(OmsConfig.loadDefaults(), null, dataSource));
        }

        assertEquals("TYCHE_REDIS_NAMESPACE is required for OMS_AUTH_MODE=tyche; "
                        + "use Tyche redis.domain + ':' + tyche.account",
                error.getMessage());
    }

    @Test
    void tycheApiKeyModeRequiresPostgresUserMapping() {
        System.setProperty("oms.auth.mode", "tyche-api-key");
        System.setProperty("oms.api.keys", "maker-secret:910001:MARKET_MAKER");
        System.setProperty("tyche.redis.namespace", "flydex:tyche:mainnet");

        IllegalStateException error = assertThrows(IllegalStateException.class,
                () -> OmsApplication.buildAuthProvider(OmsConfig.loadDefaults(), null, null));

        assertEquals("OMS_AUTH_MODE=tyche-api-key requires PostgreSQL for Tyche-to-OMS user mapping",
                error.getMessage());
    }

    @Test
    void tycheApiKeyModeRequiresRedisNamespace() {
        System.setProperty("oms.auth.mode", "tyche-api-key");
        System.setProperty("oms.api.keys", "maker-secret:910001:MARKET_MAKER");

        IllegalArgumentException error;
        try (HikariDataSource dataSource = new HikariDataSource()) {
            error = assertThrows(IllegalArgumentException.class,
                    () -> OmsApplication.buildAuthProvider(OmsConfig.loadDefaults(), null, dataSource));
        }

        assertEquals("TYCHE_REDIS_NAMESPACE is required for OMS_AUTH_MODE=tyche-api-key; "
                        + "use Tyche redis.domain + ':' + tyche.account",
                error.getMessage());
    }

    @Test
    void tycheApiKeyModeRequiresAtLeastOneApiKey() {
        System.setProperty("oms.auth.mode", "tyche-api-key");
        System.setProperty("tyche.redis.namespace", "flydex:tyche:mainnet");

        IllegalArgumentException error;
        try (HikariDataSource dataSource = new HikariDataSource()) {
            error = assertThrows(IllegalArgumentException.class,
                    () -> OmsApplication.buildAuthProvider(OmsConfig.loadDefaults(), null, dataSource));
        }

        assertEquals("OMS_AUTH_MODE=tyche-api-key requires OMS_API_KEYS or OMS_API_KEYS_FILE",
                error.getMessage());
    }

    @Test
    void stopClosesAuthenticationProviderResources() throws Exception {
        OmsApplication application = new OmsApplication();
        AtomicBoolean closed = new AtomicBoolean();
        Field field = OmsApplication.class.getDeclaredField("authProviderCloseable");
        field.setAccessible(true);
        field.set(application, (AutoCloseable) () -> closed.set(true));

        application.stop();

        assertEquals(true, closed.get());
    }
}
