-- 0050_mesh_native_keys (PostgreSQL): the static public key of each mesh
-- member and device in its region's network, from which Control builds the
-- network's native_keys (docs/design/mesh-regions). 32 bytes of X25519 in
-- standard base64; additive, NULL until reported.
ALTER TABLE "v5_mesh_member" ADD "static_key" varchar(44);
ALTER TABLE "v5_mesh_device" ADD "static_key" varchar(44);
