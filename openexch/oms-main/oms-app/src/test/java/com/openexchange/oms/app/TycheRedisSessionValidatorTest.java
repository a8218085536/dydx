// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.AuthenticationException;
import io.lettuce.core.api.sync.RedisCommands;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class TycheRedisSessionValidatorTest {

    private static final String ADDRESS = "0x" + "ab".repeat(20);

    @Test
    void validatesSessionAndUserHashesUsingTycheNamespace() throws Exception {
        @SuppressWarnings("unchecked")
        RedisCommands<String, String> redis = mock(RedisCommands.class);
        when(redis.hgetall("flydex:tyche:mainnet:auth:session:session-token"))
                .thenReturn(Map.of("userId", "tyche-user-1"));
        when(redis.hgetall("flydex:tyche:mainnet:auth:user:" + ADDRESS))
                .thenReturn(Map.of("userId", "tyche-user-1", "necoAddress", ADDRESS));

        TycheRedisSessionValidator validator =
                new TycheRedisSessionValidator(redis, "flydex:tyche:mainnet::");

        assertEquals(new TycheSession("tyche-user-1", ADDRESS),
                validator.validate("session-token", ADDRESS.toUpperCase()));
    }

    @Test
    void rejectsUnknownSessionToken() {
        @SuppressWarnings("unchecked")
        RedisCommands<String, String> redis = mock(RedisCommands.class);
        when(redis.hgetall("namespace:auth:session:missing-token")).thenReturn(Map.of());
        TycheRedisSessionValidator validator = new TycheRedisSessionValidator(redis, "namespace");

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> validator.validate("missing-token", ADDRESS));

        assertEquals("Unknown Tyche session token", error.getMessage());
    }

    @Test
    void rejectsUserRecordWithoutBoundNecoAddress() {
        @SuppressWarnings("unchecked")
        RedisCommands<String, String> redis = mock(RedisCommands.class);
        when(redis.hgetall("namespace:auth:session:session-token"))
                .thenReturn(Map.of("userId", "tyche-user-1"));
        when(redis.hgetall("namespace:auth:user:" + ADDRESS))
                .thenReturn(Map.of("userId", "tyche-user-1"));
        TycheRedisSessionValidator validator = new TycheRedisSessionValidator(redis, "namespace");

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> validator.validate("session-token", ADDRESS));

        assertEquals("Tyche user record NECO address missing", error.getMessage());
    }

    @Test
    void rejectsSessionBelongingToAnotherUser() {
        @SuppressWarnings("unchecked")
        RedisCommands<String, String> redis = mock(RedisCommands.class);
        when(redis.hgetall("namespace:auth:session:session-token"))
                .thenReturn(Map.of("userId", "tyche-user-1"));
        when(redis.hgetall("namespace:auth:user:" + ADDRESS))
                .thenReturn(Map.of("userId", "tyche-user-2", "necoAddress", ADDRESS));
        TycheRedisSessionValidator validator = new TycheRedisSessionValidator(redis, "namespace");

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> validator.validate("session-token", ADDRESS));

        assertEquals("Tyche session user does not match NECO address", error.getMessage());
    }

    @Test
    void hidesRedisFailuresBehindAuthenticationUnavailable() {
        @SuppressWarnings("unchecked")
        RedisCommands<String, String> redis = mock(RedisCommands.class);
        when(redis.hgetall("namespace:auth:session:session-token"))
                .thenThrow(new IllegalStateException("redis down"));
        TycheRedisSessionValidator validator = new TycheRedisSessionValidator(redis, "namespace");

        AuthenticationException error = assertThrows(AuthenticationException.class,
                () -> validator.validate("session-token", ADDRESS));

        assertEquals("Tyche session validation unavailable", error.getMessage());
    }
}
