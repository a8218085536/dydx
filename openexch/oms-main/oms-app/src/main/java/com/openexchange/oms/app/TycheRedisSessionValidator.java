// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.AuthenticationException;
import io.lettuce.core.RedisClient;
import io.lettuce.core.api.StatefulRedisConnection;
import io.lettuce.core.api.sync.RedisCommands;

import java.util.Locale;
import java.util.Map;
import java.util.Objects;

/** Validates Tyche session and wallet records stored in Tyche Redis. */
public final class TycheRedisSessionValidator implements TycheSessionValidator, AutoCloseable {

    private final RedisClient client;
    private final StatefulRedisConnection<String, String> connection;
    private final RedisCommands<String, String> redis;
    private final String namespace;

    public TycheRedisSessionValidator(String redisUri, String namespace) {
        this.client = RedisClient.create(redisUri);
        this.connection = client.connect();
        this.redis = connection.sync();
        this.namespace = trimTrailingColon(namespace);
    }

    TycheRedisSessionValidator(RedisCommands<String, String> redis, String namespace) {
        this.client = null;
        this.connection = null;
        this.redis = Objects.requireNonNull(redis, "redis");
        this.namespace = trimTrailingColon(namespace);
    }

    @Override
    public TycheSession validate(String sessionToken, String necoAddress)
            throws AuthenticationException {
        try {
            String token = sessionToken == null ? "" : sessionToken.trim();
            if (token.isEmpty()) {
                throw new AuthenticationException("Missing Tyche bearer token");
            }

            String normalizedAddress = necoAddress.toLowerCase(Locale.ROOT);
            Map<String, String> session = redis.hgetall(sessionKey(token));
            String tycheUserId = session.get("userId");
            if (tycheUserId == null || tycheUserId.isBlank()) {
                throw new AuthenticationException("Unknown Tyche session token");
            }

            Map<String, String> user = redis.hgetall(userKey(normalizedAddress));
            if (user.isEmpty()) {
                throw new AuthenticationException(
                        "Tyche user record not found for NECO address");
            }
            if (!tycheUserId.equals(user.get("userId"))) {
                throw new AuthenticationException(
                        "Tyche session user does not match NECO address");
            }
            String storedAddress = user.get("necoAddress");
            if (storedAddress == null || storedAddress.isBlank()) {
                throw new AuthenticationException("Tyche user record NECO address missing");
            }
            if (!storedAddress.equalsIgnoreCase(normalizedAddress)) {
                throw new AuthenticationException("Tyche user record NECO address mismatch");
            }
            return new TycheSession(tycheUserId, normalizedAddress);
        } catch (AuthenticationException e) {
            throw e;
        } catch (Exception e) {
            throw new AuthenticationException("Tyche session validation unavailable");
        }
    }

    String sessionKey(String token) {
        return namespace + ":auth:session:" + token;
    }

    String userKey(String necoAddress) {
        return namespace + ":auth:user:" + necoAddress.toLowerCase(Locale.ROOT);
    }

    private static String trimTrailingColon(String value) {
        String normalized = value == null ? "" : value.trim();
        while (normalized.endsWith(":")) {
            normalized = normalized.substring(0, normalized.length() - 1);
        }
        return normalized;
    }

    @Override
    public void close() {
        if (connection != null) {
            connection.close();
        }
        if (client != null) {
            client.shutdown();
        }
    }
}
