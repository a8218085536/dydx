// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.persistence;

import com.zaxxer.hikari.HikariDataSource;
import org.junit.jupiter.api.Test;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.startsWith;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class PostgresTycheUserMappingRepositoryTest {

    @Test
    void returnsExistingMappingWithoutWriting() throws Exception {
        HikariDataSource dataSource = mock(HikariDataSource.class);
        Connection connection = mock(Connection.class);
        PreparedStatement select = mock(PreparedStatement.class);
        ResultSet resultSet = mock(ResultSet.class);
        when(dataSource.getConnection()).thenReturn(connection);
        when(connection.prepareStatement(anyString())).thenReturn(select);
        when(select.executeQuery()).thenReturn(resultSet);
        when(resultSet.next()).thenReturn(true);
        when(resultSet.getString("neco_address")).thenReturn("0x" + "ab".repeat(20));
        when(resultSet.getLong("oms_user_id")).thenReturn(1_000_001L);

        PostgresTycheUserMappingRepository repository =
                new PostgresTycheUserMappingRepository(dataSource);
        long userId = repository.findOrCreateOmsUserId(
                "tyche-user-1", "0x" + "AB".repeat(20));

        assertEquals(1_000_001L, userId);
        verify(select).setString(1, "tyche-user-1");
        verify(connection, never()).prepareStatement(startsWith("INSERT"));
    }

    @Test
    void createsMappingWhenNoneExists() throws Exception {
        HikariDataSource dataSource = mock(HikariDataSource.class);
        Connection connection = mock(Connection.class);
        PreparedStatement select = mock(PreparedStatement.class);
        PreparedStatement insert = mock(PreparedStatement.class);
        ResultSet missing = mock(ResultSet.class);
        ResultSet inserted = mock(ResultSet.class);
        when(dataSource.getConnection()).thenReturn(connection);
        when(connection.prepareStatement(startsWith("SELECT")))
                .thenReturn(select);
        when(connection.prepareStatement(startsWith("INSERT")))
                .thenReturn(insert);
        when(select.executeQuery()).thenReturn(missing);
        when(missing.next()).thenReturn(false);
        when(insert.executeQuery()).thenReturn(inserted);
        when(inserted.next()).thenReturn(true);
        when(inserted.getString("neco_address")).thenReturn("0x" + "ab".repeat(20));
        when(inserted.getLong("oms_user_id")).thenReturn(1_000_001L);

        PostgresTycheUserMappingRepository repository =
                new PostgresTycheUserMappingRepository(dataSource);
        long userId = repository.findOrCreateOmsUserId(
                "tyche-user-1", "0x" + "AB".repeat(20));

        assertEquals(1_000_001L, userId);
        verify(insert).setString(1, "tyche-user-1");
        verify(insert).setString(2, "0x" + "ab".repeat(20));
    }

    @Test
    void rejectsRemappingTycheUserToDifferentWallet() throws Exception {
        HikariDataSource dataSource = mock(HikariDataSource.class);
        Connection connection = mock(Connection.class);
        PreparedStatement select = mock(PreparedStatement.class);
        ResultSet resultSet = mock(ResultSet.class);
        when(dataSource.getConnection()).thenReturn(connection);
        when(connection.prepareStatement(anyString())).thenReturn(select);
        when(select.executeQuery()).thenReturn(resultSet);
        when(resultSet.next()).thenReturn(true);
        when(resultSet.getString("neco_address")).thenReturn("0x" + "cd".repeat(20));

        PostgresTycheUserMappingRepository repository =
                new PostgresTycheUserMappingRepository(dataSource);

        PersistenceException error = assertThrows(PersistenceException.class,
                () -> repository.findOrCreateOmsUserId(
                        "tyche-user-1", "0x" + "ab".repeat(20)));
        assertEquals("Tyche user is already mapped to a different NECO address", error.getMessage());
    }

    @Test
    void readsMappingAgainAfterConcurrentInsertWins() throws Exception {
        HikariDataSource dataSource = mock(HikariDataSource.class);
        Connection connection = mock(Connection.class);
        PreparedStatement firstSelect = mock(PreparedStatement.class);
        PreparedStatement insert = mock(PreparedStatement.class);
        PreparedStatement secondSelect = mock(PreparedStatement.class);
        ResultSet initiallyMissing = mock(ResultSet.class);
        ResultSet insertLostRace = mock(ResultSet.class);
        ResultSet existing = mock(ResultSet.class);
        when(dataSource.getConnection()).thenReturn(connection);
        when(connection.prepareStatement(startsWith("SELECT")))
                .thenReturn(firstSelect, secondSelect);
        when(connection.prepareStatement(startsWith("INSERT")))
                .thenReturn(insert);
        when(firstSelect.executeQuery()).thenReturn(initiallyMissing);
        when(initiallyMissing.next()).thenReturn(false);
        when(insert.executeQuery()).thenReturn(insertLostRace);
        when(insertLostRace.next()).thenReturn(false);
        when(secondSelect.executeQuery()).thenReturn(existing);
        when(existing.next()).thenReturn(true);
        when(existing.getString("neco_address")).thenReturn("0x" + "ab".repeat(20));
        when(existing.getLong("oms_user_id")).thenReturn(1_000_001L);

        PostgresTycheUserMappingRepository repository =
                new PostgresTycheUserMappingRepository(dataSource);

        assertEquals(1_000_001L, repository.findOrCreateOmsUserId(
                "tyche-user-1", "0x" + "ab".repeat(20)));
    }
}
