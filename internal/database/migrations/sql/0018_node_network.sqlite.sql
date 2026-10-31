-- 0018_node_network: each device's own network settings
-- (v4_kernel_node_network): the interface, source and gateway its
-- automatic network model egresses leave by, and its hop ingress port.
CREATE TABLE `v4_kernel_node_network` (`node_ref` text,`interface` text NOT NULL DEFAULT "",`source_v4` text NOT NULL DEFAULT "",`gateway_v4` text NOT NULL DEFAULT "",`source_v6` text NOT NULL DEFAULT "",`gateway_v6` text NOT NULL DEFAULT "",`hop_port` integer NOT NULL DEFAULT 0,`updated_at` datetime NOT NULL,PRIMARY KEY (`node_ref`));
