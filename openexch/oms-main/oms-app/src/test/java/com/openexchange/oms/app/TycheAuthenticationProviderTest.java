// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.AuthenticationException;
import com.openexchange.oms.api.auth.AuthenticationProvider;
import com.openexchange.oms.api.auth.Principal;
import org.junit.jupiter.api.Test;

import java.util.Map;
import java.util.TreeMap;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class TycheAuthenticationProviderTest {

    private static final String ADDRESS = "0x" + "A1".repeat(20);
    private static final String NORMALIZED_ADDRESS = ADDRESS.toLowerCase();

    @Test
    void authenticatesTycheSessionAndMapsItToOmsUser() throws Exception {
        AtomicReference<String> validatedToken = new AtomicReference<>();
        AtomicReference<String> validatedAddress = new AtomicReference<>();
        AtomicReference<String> mappedTycheUserId = new AtomicReference<>();

        TycheSessionValidator sessions = (token, address) -> {
            validatedToken.set(token);
            validatedAddress.set(address);
            return new TycheSession("tyche-user-1", address);
        };
        TycheUserMapper users = (tycheUserId, address) -> {
            mappedTycheUserId.set(tycheUserId);
            assertEquals(NORMALIZED_ADDRESS, address);
            return 1_000_001L;
        };

        TycheAuthenticationProvider provider = new TycheAuthenticationProvider(sessions, users);
        Principal principal = provider.authenticate(headers(
                "Authorization", "Bearer session-token",
                "X-Neco-Address", ADDRESS,
                "X-Tyche-User-Id", "tyche-user-1"));

        assertEquals("session-token", validatedToken.get());
        assertEquals(NORMALIZED_ADDRESS, validatedAddress.get());
        assertEquals("tyche-user-1", mappedTycheUserId.get());
        assertEquals("tyche:tyche-user-1", principal.subject());
        assertEquals(1_000_001L, principal.userId());
        assertTrue(principal.roles().isEmpty());
    }

    @Test
    void rejectsUserIdHeaderThatDoesNotMatchSession() {
        TycheAuthenticationProvider provider = new TycheAuthenticationProvider(
                (token, address) -> new TycheSession("real-user", address),
                (tycheUserId, address) -> 1L);

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> provider.authenticate(headers(
                        "Authorization", "Bearer session-token",
                        "X-Neco-Address", ADDRESS,
                        "X-Tyche-User-Id", "other-user")));

        assertEquals("Tyche userId header does not match session", error.getMessage());
    }

    @Test
    void rejectsInvalidNecoAddressBeforeSessionLookup() {
        TycheAuthenticationProvider provider = new TycheAuthenticationProvider(
                (token, address) -> new TycheSession("user", address),
                (tycheUserId, address) -> 1L);

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> provider.authenticate(headers(
                        "Authorization", "Bearer session-token",
                        "X-Neco-Address", "not-an-address")));

        assertEquals("Invalid NECO EVM address", error.getMessage());
    }

    @Test
    void rejectsApiKeyHeaderBecauseTycheRequiresBearerToken() {
        TycheAuthenticationProvider provider = new TycheAuthenticationProvider(
                (token, address) -> new TycheSession("user", address),
                (tycheUserId, address) -> 1L);

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> provider.authenticate(headers(
                        "X-API-Key", "session-token",
                        "X-Neco-Address", ADDRESS)));

        assertEquals("Missing Tyche bearer token", error.getMessage());
    }

    @Test
    void closesCloseableSessionValidator() throws Exception {
        AtomicBoolean closed = new AtomicBoolean();
        class CloseableValidator implements TycheSessionValidator, AutoCloseable {
            @Override
            public TycheSession validate(String token, String address) {
                return new TycheSession("user", address);
            }

            @Override
            public void close() {
                closed.set(true);
            }
        }

        TycheAuthenticationProvider provider = new TycheAuthenticationProvider(
                new CloseableValidator(), (tycheUserId, address) -> 1L);
        provider.close();

        assertTrue(closed.get());
    }

    private static AuthenticationProvider.Headers headers(String... nameValuePairs) {
        Map<String, String> values = new TreeMap<>(String.CASE_INSENSITIVE_ORDER);
        for (int i = 0; i < nameValuePairs.length; i += 2) {
            values.put(nameValuePairs[i], nameValuePairs[i + 1]);
        }
        return values::get;
    }
}
