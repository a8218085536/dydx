// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.persistence;

import com.zaxxer.hikari.HikariDataSource;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Locale;

/** Durable mapping from Tyche string identities to numeric OMS user IDs. */
public final class PostgresTycheUserMappingRepository {

    private static final String FIND = """
            SELECT oms_user_id, neco_address
            FROM tyche_user_mappings
            WHERE tyche_user_id = ?
            """;

    private static final String INSERT = """
            INSERT INTO tyche_user_mappings (tyche_user_id, neco_address)
            VALUES (?, ?)
            ON CONFLICT (tyche_user_id) DO NOTHING
            RETURNING oms_user_id, neco_address
            """;

    private final HikariDataSource dataSource;

    public PostgresTycheUserMappingRepository(HikariDataSource dataSource) {
        this.dataSource = dataSource;
    }

    public long findOrCreateOmsUserId(String tycheUserId, String necoAddress) {
        String normalizedAddress = necoAddress.toLowerCase(Locale.ROOT);
        try (Connection connection = dataSource.getConnection()) {
            Mapping existing = find(connection, tycheUserId);
            if (existing != null) {
                return verifyAddress(existing, normalizedAddress);
            }

            Mapping inserted = insert(connection, tycheUserId, normalizedAddress);
            if (inserted != null) {
                return verifyAddress(inserted, normalizedAddress);
            }

            // Another request inserted the same Tyche user after our first read.
            Mapping concurrent = find(connection, tycheUserId);
            if (concurrent == null) {
                throw new PersistenceException("Failed to map Tyche user");
            }
            return verifyAddress(concurrent, normalizedAddress);
        } catch (SQLException e) {
            throw new PersistenceException("Failed to map Tyche user", e);
        }
    }

    private static Mapping find(Connection connection, String tycheUserId) throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement(FIND)) {
            statement.setString(1, tycheUserId);
            try (ResultSet resultSet = statement.executeQuery()) {
                return resultSet.next() ? mapping(resultSet) : null;
            }
        }
    }

    private static Mapping insert(
            Connection connection, String tycheUserId, String normalizedAddress)
            throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement(INSERT)) {
            statement.setString(1, tycheUserId);
            statement.setString(2, normalizedAddress);
            try (ResultSet resultSet = statement.executeQuery()) {
                return resultSet.next() ? mapping(resultSet) : null;
            }
        }
    }

    private static Mapping mapping(ResultSet resultSet) throws SQLException {
        return new Mapping(
                resultSet.getLong("oms_user_id"), resultSet.getString("neco_address"));
    }

    private static long verifyAddress(Mapping mapping, String normalizedAddress) {
        if (!normalizedAddress.equals(mapping.necoAddress())) {
            throw new PersistenceException(
                    "Tyche user is already mapped to a different NECO address");
        }
        return mapping.omsUserId();
    }

    private record Mapping(long omsUserId, String necoAddress) {}
}
