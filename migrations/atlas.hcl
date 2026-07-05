env "local" {
  src = "file://migrations/schema.pg.hcl"
  dev = "docker://postgres/16/dev?search_path=public"
  url = getenv("DATABASE_URL")
  migration {
    dir = "file://migrations"
  }
  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}
