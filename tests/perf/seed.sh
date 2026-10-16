#!/usr/bin/env bash
# 压测造数（p6 §2.1）：万级用户/万级内容/千级圈子/百级猫立方，写入指定库（应为独立压测库）。
# 用法：CATCUBE_PERF_DATABASE_URL=postgres://... ./tests/perf/seed.sh
# 幂等：ID 段固定（>= 9e17），重复执行按主键冲突忽略。
set -euo pipefail

: "${CATCUBE_PERF_DATABASE_URL:?需设置 CATCUBE_PERF_DATABASE_URL（压测库）}"
PSQL=(psql "$CATCUBE_PERF_DATABASE_URL" -v ON_ERROR_STOP=1 -q)

echo "[seed] 用户 10000 / 名片 10000 …"
"${PSQL[@]}" <<'SQL'
INSERT INTO idn_user (id, phone_hmac, nickname, status, identity_type, created_at, updated_at)
SELECT 900000000000000000 + g,
       digest('perf-phone-' || g, 'sha256'),
       '压测用户' || g, 0, 1, now(), now()
FROM generate_series(1, 10000) AS g
ON CONFLICT (id) DO NOTHING;

INSERT INTO prf_card (user_id, name, region_code, industry_codes, need_codes, offer_codes, profile_tier, created_at, updated_at)
SELECT 900000000000000000 + g, '压测用户' || g, '310000', 'A01', 'N1', 'O1', 2, now(), now()
FROM generate_series(1, 10000) AS g
ON CONFLICT (user_id) DO NOTHING;
SQL

echo "[seed] 圈子 1000（含成员 5/圈）…"
"${PSQL[@]}" <<'SQL'
INSERT INTO cir_circle (id, name, intro, type, duration_days, end_at, owner_user_id, member_count, status, created_at, updated_at)
SELECT 910000000000000000 + g, '压测圈' || g, '压测用圈', 2, 30, now() + interval '30 days',
       900000000000000000 + ((g % 10000) + 1), 5, 0, now(), now()
FROM generate_series(1, 1000) AS g
ON CONFLICT (id) DO NOTHING;

INSERT INTO cir_member (id, circle_id, user_id, role, status, joined_at, created_at, updated_at)
SELECT 911000000000000000 + g * 10 + m, 910000000000000000 + g,
       900000000000000000 + (((g * 7 + m) % 10000) + 1), CASE WHEN m = 1 THEN 1 ELSE 2 END, 1, now(), now(), now()
FROM generate_series(1, 1000) AS g, generate_series(1, 5) AS m
ON CONFLICT (id) DO NOTHING;
SQL

echo "[seed] 猫立方 100（含成员 3/立方）…"
"${PSQL[@]}" <<'SQL'
INSERT INTO cub_cube (id, name, leader_id, honey_id, plan_level, plan_expire_at, status, created_at, updated_at)
SELECT 920000000000000000 + g, '压测立方' || g,
       900000000000000000 + ((g % 10000) + 1), 900000000000000000 + (((g + 1) % 10000) + 1),
       1, now() + interval '365 days', 0, now(), now()
FROM generate_series(1, 100) AS g
ON CONFLICT (id) DO NOTHING;

INSERT INTO cub_member (id, cube_id, user_id, role, status, joined_at, created_at, updated_at)
SELECT 921000000000000000 + g * 10 + m, 920000000000000000 + g,
       900000000000000000 + (((g * 3 + m) % 10000) + 1), CASE WHEN m = 1 THEN 1 ELSE 4 END, 1, now(), now(), now()
FROM generate_series(1, 100) AS g, generate_series(1, 3) AS m
ON CONFLICT (id) DO NOTHING;
SQL

echo "[seed] 内容 10000（项目）+ 搜索投影 …"
"${PSQL[@]}" <<'SQL'
INSERT INTO cnt_item (id, kind, owner_type, owner_id, title, region_code, tagset, status, published_at, created_at, updated_at)
SELECT 930000000000000000 + g, 'project', 'user', 900000000000000000 + ((g % 10000) + 1),
       '压测项目' || g, '310000', '{"industry":["A01"],"region":"310000","need":["N1"]}'::jsonb, 1, now(), now(), now()
FROM generate_series(1, 10000) AS g
ON CONFLICT (id) DO NOTHING;

INSERT INTO cnt_project (item_id, amount_wan, stage, intro, contact_phone_enc)
SELECT 930000000000000000 + g, 100, '洽谈中', '压测项目简介', '\x00'::bytea
FROM generate_series(1, 10000) AS g
ON CONFLICT (item_id) DO NOTHING;

INSERT INTO sys_search_doc (doc_type, doc_id, title, doc, updated_at)
SELECT 'content.project', (930000000000000000 + g)::text, '压测项目' || g,
       jsonb_build_object('title', '压测项目' || g, 'region', '310000'), now()
FROM generate_series(1, 10000) AS g
ON CONFLICT (doc_type, doc_id) DO NOTHING;
SQL

"${PSQL[@]}" -tAc "SELECT '[seed] 完成：users='||(SELECT count(*) FROM idn_user)||' contents='||(SELECT count(*) FROM cnt_item)||' circles='||(SELECT count(*) FROM cir_circle)||' cubes='||(SELECT count(*) FROM cub_cube)"
