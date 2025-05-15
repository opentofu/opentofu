removed {
  from = test.foo
  lifecycle {
    destroy = false
  }
}

removed {
  from = test.foo
  lifecycle {
    destroy = true
  }
}

removed {
  from = module.a
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.a
  lifecycle {
    destroy = true
  }
}

removed {
  from = test.foo
  lifecycle {
    destroy = false
  }
}
