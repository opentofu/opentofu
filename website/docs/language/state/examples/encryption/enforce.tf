terraform {
    encryption {
        statefile {
            enforce = true
        }
        planfile {
            enforce = true
        }
        backend {
            enforce = true
        }
    }
}