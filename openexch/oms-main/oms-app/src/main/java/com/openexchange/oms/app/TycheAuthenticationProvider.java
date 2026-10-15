// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.AuthenticationException;
import com.openexchange.oms.api.auth.AuthenticationProvider;
import com.openexchange.oms.api.auth.Principal;

import java.util.Locale;
import java.util.Set;

/** Authenticates OMS callers using Tyche wallet-login sessions. */
public final class TycheAuthenticationProvider implements AuthenticationProvider, AutoCloseable {

    public static final String NECO_ADDRESS_HEADER = "X-Neco-Address";
    public static final String TYCHE_USER_ID_HEADER = "X-Tyche-User-Id";

    private final TycheSessionValidator sessions;
    private final TycheUserMapper users;

    public TycheAuthenticationProvider(TycheSessionValidator sessions, TycheUserMapper users) {
        this.sessions = sessions;
        this.users = users;
    }

    @Override
    public Principal authenticate(Headers headers) throws AuthenticationException {
        String token = extractBearerToken(headers);
        if (token == null || token.isBlank()) {
            throw new AuthenticationException("Missing Tyche bearer token");
        }

        String necoAddress = normalizeAddress(headers.get(NECO_ADDRESS_HEADER));
        if (necoAddress == null) {
            throw new AuthenticationException("Missing X-Neco-Address header");
        }

        TycheSession session = sessions.validate(token, necoAddress);
        String requestedTycheUserId = trimToNull(headers.get(TYCHE_USER_ID_HEADER));
        if (requestedTycheUserId != null
                && !requestedTycheUserId.equals(session.tycheUserId())) {
            throw new AuthenticationException("Tyche userId header does not match session");
        }

        String sessionAddress = normalizeAddress(session.necoAddress());
        if (!necoAddress.equals(sessionAddress)) {
            throw new AuthenticationException(
                    "Tyche session NECO address does not match X-Neco-Address");
        }

        long omsUserId = users.findOrCreateOmsUserId(session.tycheUserId(), necoAddress);
        return new Principal("tyche:" + session.tycheUserId(), omsUserId, Set.of());
    }

    private static String extractBearerToken(Headers headers) {
        String authorization = headers.get("Authorization");
        if (authorization == null
                || !authorization.regionMatches(true, 0, "Bearer ", 0, 7)) {
            return null;
        }
        return authorization.substring(7).trim();
    }

    private static String trimToNull(String value) {
        return value == null || value.isBlank() ? null : value.trim();
    }

    private static String normalizeAddress(String value) throws AuthenticationException {
        if (value == null || value.isBlank()) {
            return null;
        }
        String normalized = value.trim().toLowerCase(Locale.ROOT);
        if (!isEvmAddress(normalized)) {
            throw new AuthenticationException("Invalid NECO EVM address");
        }
        return normalized;
    }

    private static boolean isEvmAddress(String value) {
        if (value.length() != 42 || !value.startsWith("0x")) {
            return false;
        }
        for (int i = 2; i < value.length(); i++) {
            char c = value.charAt(i);
            if ((c < '0' || c > '9') && (c < 'a' || c > 'f')) {
                return false;
            }
        }
        return true;
    }

    @Override
    public void close() throws Exception {
        if (sessions instanceof AutoCloseable closeable) {
            closeable.close();
        }
    }
}
