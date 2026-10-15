// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.persistence;

import org.junit.jupiter.api.Test;

import java.io.InputStream;
import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class TycheUserMappingMigrationTest {

    @Test
    void migrationCreatesDurableUniqueMappings() throws Exception {
        try (InputStream stream = getClass().getResourceAsStream(
                "/db/migration/V005__tyche_user_mappings.sql")) {
            assertNotNull(stream, "Tyche user mapping migration must be packaged");
            String sql = new String(stream.readAllBytes(), StandardCharsets.UTF_8);
            assertTrue(sql.contains("CREATE TABLE IF NOT EXISTS tyche_user_mappings"));
            assertTrue(sql.contains("tyche_user_id TEXT NOT NULL UNIQUE"));
            assertTrue(sql.contains("neco_address TEXT NOT NULL UNIQUE"));
        }
    }
}
