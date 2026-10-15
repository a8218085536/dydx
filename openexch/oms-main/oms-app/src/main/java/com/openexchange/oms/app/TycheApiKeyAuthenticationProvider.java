// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.AuthenticationException;
import com.openexchange.oms.api.auth.AuthenticationProvider;
import com.openexchange.oms.api.auth.Principal;

import java.util.concurrent.atomic.AtomicBoolean;

/** Accepts dedicated API keys alongside the existing Tyche wallet sessions. */
public final class TycheApiKeyAuthenticationProvider
        implements AuthenticationProvider, AutoCloseable {

    private final AuthenticationProvider apiKeys;
    private final AuthenticationProvider tyche;
    private final AtomicBoolean closed = new AtomicBoolean();

    public TycheApiKeyAuthenticationProvider(
            AuthenticationProvider apiKeys, AuthenticationProvider tyche) {
        this.apiKeys = apiKeys;
        this.tyche = tyche;
    }

    @Override
    public Principal authenticate(Headers headers) throws AuthenticationException {
        String explicitApiKey = headers.get("X-API-Key");
        if (explicitApiKey != null && !explicitApiKey.isBlank()) {
            Headers apiKeyOnlyHeaders = name -> {
                if ("Authorization".equalsIgnoreCase(name)) {
                    return null;
                }
                if ("X-API-Key".equalsIgnoreCase(name)) {
                    return explicitApiKey;
                }
                return headers.get(name);
            };
            return apiKeys.authenticate(apiKeyOnlyHeaders);
        }

        try {
            return apiKeys.authenticate(headers);
        } catch (AuthenticationException ignored) {
            return tyche.authenticate(headers);
        }
    }

    @Override
    public void close() throws Exception {
        if (!closed.compareAndSet(false, true)) {
            return;
        }
        Exception first = close(apiKeys, null);
        first = close(tyche, first);
        if (first != null) {
            throw first;
        }
    }

    private static Exception close(AuthenticationProvider provider, Exception first) {
        if (!(provider instanceof AutoCloseable closeable)) {
            return first;
        }
        try {
            closeable.close();
        } catch (Exception e) {
            if (first == null) {
                return e;
            }
            first.addSuppressed(e);
        }
        return first;
    }
}
