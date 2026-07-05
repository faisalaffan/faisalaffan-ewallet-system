table "ledger_entries" {
  schema = schema.public
  column "entry_id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "wallet_id" {
    null = false
    type = uuid
  }
  column "entry_type" {
    null = false
    type = character_varying(20)
  }
  column "amount" {
    null = false
    type = numeric(20,2)
  }
  column "currency" {
    null = false
    type = character(3)
  }
  column "balance_after" {
    null = false
    type = numeric(20,2)
  }
  column "reference_id" {
    null = true
    type = character_varying(36)
  }
  column "idempotency_key" {
    null = false
    type = character_varying(64)
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.entry_id]
  }
  foreign_key "fk_ledger_wallet" {
    columns     = [column.wallet_id]
    ref_columns = [table.wallets.column.id]
    on_update   = NO_ACTION
    on_delete   = NO_ACTION
  }
  index "idx_ledger_wallet_created" {
    columns = [column.wallet_id, column.created_at]
  }
  unique "uq_idempotency_key" {
    columns = [column.idempotency_key]
  }
}
table "wallets" {
  schema = schema.public
  column "id" {
    null    = false
    type    = uuid
    default = sql("gen_random_uuid()")
  }
  column "owner_id" {
    null = false
    type = text
  }
  column "currency" {
    null = false
    type = character(3)
  }
  column "balance" {
    null    = false
    type    = numeric(20,2)
    default = 0
  }
  column "status" {
    null    = false
    type    = character_varying(20)
    default = "ACTIVE"
  }
  column "created_at" {
    null = true
    type = timestamptz
  }
  column "updated_at" {
    null = true
    type = timestamptz
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_wallets_owner_id" {
    columns = [column.owner_id]
  }
  unique "uq_wallets_owner_currency" {
    columns = [column.owner_id, column.currency]
  }
}
schema "public" {
  comment = "standard public schema"
}
