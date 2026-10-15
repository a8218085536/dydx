// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.ApiKeyAuthenticationProvider;
import com.openexchange.oms.api.auth.AuthenticationException;
import com.openexchange.oms.api.auth.AuthenticationProvider;
import com.openexchange.oms.api.auth.Principal;
import org.junit.jupiter.api.Test;

import java.util.Map;
import java.util.Set;
import java.util.TreeMap;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class TycheApiKeyAuthenticationProviderTest {

    private static final String API_KEY = "maker-secret-910001";
    private static final Principal TYCHE_PRINCIPAL =
            new Principal("tyche:user-1", 1_000_001L, Set.of());

    @Test
    void xApiKeySelectsApiKeyProvider() throws Exception {
        AtomicInteger tycheCalls = new AtomicInteger();
        AuthenticationProvider tyche = headers -> {
            tycheCalls.incrementAndGet();
            return TYCHE_PRINCIPAL;
        };
        TycheApiKeyAuthenticationProvider provider = provider(tyche);

        Principal principal = provider.authenticate(headers(
                "Authorization", "Bearer valid-tyche-session",
                "X-API-Key", API_KEY));

        assertEquals(910001L, principal.userId());
        assertEquals(Set.of("MARKET_MAKER"), principal.roles());
        assertEquals(0, tycheCalls.get());
    }

    @Test
    void invalidExplicitApiKeyNeverFallsBackToTyche() {
        AtomicInteger tycheCalls = new AtomicInteger();
        AuthenticationProvider tyche = headers -> {
            tycheCalls.incrementAndGet();
            return TYCHE_PRINCIPAL;
        };
        TycheApiKeyAuthenticationProvider provider = provider(tyche);

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> provider.authenticate(headers(
                        "Authorization", "Bearer valid-tyche-session",
                        "X-API-Key", "invalid-explicit-secret")));

        assertEquals("Unknown API key", error.getMessage());
        assertFalse(error.getMessage().contains("invalid-explicit-secret"));
        assertEquals(0, tycheCalls.get());
    }

    @Test
    void bearerApiKeySupportsWebSocketAuthentication() throws Exception {
        AtomicInteger tycheCalls = new AtomicInteger();
        AuthenticationProvider tyche = headers -> {
            tycheCalls.incrementAndGet();
            return TYCHE_PRINCIPAL;
        };
        TycheApiKeyAuthenticationProvider provider = provider(tyche);

        Principal principal = provider.authenticate(headers("Authorization", "Bearer " + API_KEY));

        assertEquals(910001L, principal.userId());
        assertEquals(0, tycheCalls.get());
    }

    @Test
    void unknownBearerFallsBackToTyche() throws Exception {
        AtomicInteger tycheCalls = new AtomicInteger();
        AuthenticationProvider tyche = headers -> {
            tycheCalls.incrementAndGet();
            return TYCHE_PRINCIPAL;
        };
        TycheApiKeyAuthenticationProvider provider = provider(tyche);

        Principal principal = provider.authenticate(headers(
                "Authorization", "Bearer valid-tyche-session"));

        assertEquals(TYCHE_PRINCIPAL, principal);
        assertEquals(1, tycheCalls.get());
    }

    @Test
    void closeClosesBothProviders() throws Exception {
        CloseableProvider apiKeys = new CloseableProvider();
        CloseableProvider tyche = new CloseableProvider();
        TycheApiKeyAuthenticationProvider provider =
                new TycheApiKeyAuthenticationProvider(apiKeys, tyche);

        provider.close();

        assertTrue(apiKeys.closed.get());
        assertTrue(tyche.closed.get());
    }

    private static TycheApiKeyAuthenticationProvider provider(AuthenticationProvider tyche) {
        ApiKeyAuthenticationProvider apiKeys = ApiKeyAuthenticationProvider.parse(
                API_KEY + ":910001:MARKET_MAKER", "");
        return new TycheApiKeyAuthenticationProvider(apiKeys, tyche);
    }

    private static AuthenticationProvider.Headers headers(String... nameValuePairs) {
        Map<String, String> values = new TreeMap<>(String.CASE_INSENSITIVE_ORDER);
        for (int i = 0; i < nameValuePairs.length; i += 2) {
            values.put(nameValuePairs[i], nameValuePairs[i + 1]);
        }
        return values::get;
    }

    private static final class CloseableProvider implements AuthenticationProvider, AutoCloseable {
        private final AtomicBoolean closed = new AtomicBoolean();

        @Override
        public Principal authenticate(Headers headers) {
            return new Principal("closeable", 1L, Set.of());
        }

        @Override
        public void close() {
            closed.set(true);
        }
    }
}
