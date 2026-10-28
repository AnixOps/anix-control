-- 0016_network_model (SQLite): the device network model
-- (docs/design/network-model, internal/networkmodel): the model document
-- (v4_kernel_network_model), the ANX-Line link keys of its transports
-- sealed under Control's key-encryption key
-- (v4_kernel_network_transport_key), and each device's plan generation and
-- reported facts (v4_kernel_network_device).
CREATE TABLE `v4_kernel_network_model` (`id` integer PRIMARY KEY AUTOINCREMENT,`revision` integer NOT NULL DEFAULT 0,`document_json` text NOT NULL DEFAULT "",`updated_at` datetime NOT NULL);
CREATE TABLE `v4_kernel_network_transport_key` (`transport_id` text,`sealed_key` text NOT NULL,`created_at` datetime NOT NULL,`updated_at` datetime NOT NULL,PRIMARY KEY (`transport_id`));
CREATE TABLE `v4_kernel_network_device` (`node_ref` text,`generation` integer NOT NULL DEFAULT 0,`plan_hash` text NOT NULL DEFAULT "",`negotiated` numeric NOT NULL DEFAULT false,`capabilities_json` text NOT NULL DEFAULT "",`public_keys_json` text NOT NULL DEFAULT "",`status_json` text NOT NULL DEFAULT "",`status_at` datetime,`updated_at` datetime NOT NULL,PRIMARY KEY (`node_ref`));
