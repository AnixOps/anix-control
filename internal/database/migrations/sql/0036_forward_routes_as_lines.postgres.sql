-- 0036_forward_routes_as_lines (PostgreSQL): forward routes are lines
-- (docs/design/network-lines, step 2). v5_line, which nothing wrote yet,
-- is created again with the forward route's columns (owner, revision, the
-- route as protojson, enforcement, created_at); every route of
-- v4_kernel_forward_route moves into it and the old table goes. The line
-- columns are written from the routes after the next plan.
DROP TABLE IF EXISTS "v5_line";
CREATE TABLE "v5_line" ("id" varchar(64),"owner" varchar(64) NOT NULL DEFAULT '',"revision" bigint NOT NULL DEFAULT 0,"route" text NOT NULL DEFAULT '',"enforced" varchar(16) NOT NULL DEFAULT '',"sort" bigint NOT NULL DEFAULT 0,"name" varchar(191) NOT NULL DEFAULT '',"enabled" boolean NOT NULL DEFAULT false,"show" boolean NOT NULL DEFAULT false,"entries" text,"entry_hostname" varchar(255) NOT NULL DEFAULT '',"listen" varchar(64) NOT NULL DEFAULT '',"port" bigint NOT NULL DEFAULT 0,"l4" text,"chain" text,"exit" varchar(64) NOT NULL DEFAULT '',"rules" text,"protocol_id" bigint,"targets" text,"policy" text,"hop_balance" varchar(32) NOT NULL DEFAULT '',"rate" decimal NOT NULL DEFAULT 1,"serve_at" varchar(32) NOT NULL DEFAULT '',"created_at" timestamptz NOT NULL,"updated_at" timestamptz NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_line_protocol_id" ON "v5_line" ("protocol_id");
CREATE INDEX IF NOT EXISTS "idx_v5_line_owner" ON "v5_line" ("owner");
INSERT INTO "v5_line" ("id","owner","revision","route","enforced","created_at","updated_at")
SELECT "id","owner","revision","route_json","enforced","created_at","updated_at" FROM "v4_kernel_forward_route";
DROP TABLE "v4_kernel_forward_route";
