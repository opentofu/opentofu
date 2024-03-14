terraform {
    encryption {
        state {
            enforced = true
        }
        plan {
            enforced = true
        }
    }
}