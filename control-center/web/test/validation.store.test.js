import { describe, it, expect } from 'vitest'

// Form validation functions
const validateEmail = (email) => {
  if (!email) return { valid: false, error: 'Email is required' }
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
  if (!emailRegex.test(email)) return { valid: false, error: 'Invalid email format' }
  return { valid: true }
}

const validatePassword = (password) => {
  if (!password) return { valid: false, error: 'Password is required' }
  if (password.length < 6) return { valid: false, error: 'Password must be at least 6 characters' }
  if (password.length > 100) return { valid: false, error: 'Password is too long' }
  return { valid: true }
}

const validateRequired = (value, fieldName) => {
  if (!value || (typeof value === 'string' && value.trim() === '')) {
    return { valid: false, error: `${fieldName} is required` }
  }
  return { valid: true }
}

const validatePort = (port) => {
  const num = parseInt(port)
  if (isNaN(num)) return { valid: false, error: 'Port must be a number' }
  if (num < 1 || num > 65535) return { valid: false, error: 'Port must be between 1 and 65535' }
  return { valid: true }
}

const validateIP = (ip) => {
  if (!ip) return { valid: false, error: 'IP address is required' }
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/
  if (!ipv4Regex.test(ip)) return { valid: false, error: 'Invalid IP address format' }
  const parts = ip.split('.').map(Number)
  if (parts.some(p => p < 0 || p > 255)) {
    return { valid: false, error: 'IP address octets must be 0-255' }
  }
  return { valid: true }
}

const validateCron = (cron) => {
  if (!cron) return { valid: false, error: 'Cron expression is required' }
  const parts = cron.trim().split(/\s+/)
  if (parts.length !== 5) return { valid: false, error: 'Cron must have 5 fields' }
  return { valid: true }
}

describe('Email Validation', () => {
  it('validates correct email formats', () => {
    expect(validateEmail('user@example.com').valid).toBe(true)
    expect(validateEmail('admin@domain.org').valid).toBe(true)
    expect(validateEmail('test.user@company.co.uk').valid).toBe(true)
  })

  it('rejects invalid email formats', () => {
    expect(validateEmail('invalid').valid).toBe(false)
    expect(validateEmail('no@domain').valid).toBe(false)
    expect(validateEmail('@nodomain.com').valid).toBe(false)
  })

  it('rejects empty email', () => {
    expect(validateEmail('').valid).toBe(false)
    expect(validateEmail(null).valid).toBe(false)
  })
})

describe('Password Validation', () => {
  it('validates correct password lengths', () => {
    expect(validatePassword('123456').valid).toBe(true)
    expect(validatePassword('password123').valid).toBe(true)
  })

  it('rejects short passwords', () => {
    expect(validatePassword('12345').valid).toBe(false)
  })

  it('rejects empty passwords', () => {
    expect(validatePassword('').valid).toBe(false)
    expect(validatePassword(null).valid).toBe(false)
  })
})

describe('Required Field Validation', () => {
  it('validates non-empty strings', () => {
    expect(validateRequired('value', 'Field').valid).toBe(true)
    expect(validateRequired(123, 'Field').valid).toBe(true)
  })

  it('rejects empty values', () => {
    expect(validateRequired('', 'Field').valid).toBe(false)
    expect(validateRequired(null, 'Field').valid).toBe(false)
    expect(validateRequired(undefined, 'Field').valid).toBe(false)
  })

  it('rejects whitespace-only strings', () => {
    expect(validateRequired('   ', 'Field').valid).toBe(false)
  })

  it('includes field name in error', () => {
    const result = validateRequired('', 'Username')
    expect(result.error).toContain('Username')
  })
})

describe('Port Validation', () => {
  it('validates valid port numbers', () => {
    expect(validatePort(80).valid).toBe(true)
    expect(validatePort(443).valid).toBe(true)
    expect(validatePort(8080).valid).toBe(true)
    expect(validatePort(65535).valid).toBe(true)
  })

  it('rejects invalid port numbers', () => {
    expect(validatePort(0).valid).toBe(false)
    expect(validatePort(-1).valid).toBe(false)
    expect(validatePort(65536).valid).toBe(false)
  })

  it('rejects non-numeric values', () => {
    expect(validatePort('abc').valid).toBe(false)
  })
})

describe('IP Address Validation', () => {
  it('validates correct IPv4 addresses', () => {
    expect(validateIP('192.168.1.1').valid).toBe(true)
    expect(validateIP('10.0.0.1').valid).toBe(true)
    expect(validateIP('255.255.255.255').valid).toBe(true)
    expect(validateIP('0.0.0.0').valid).toBe(true)
  })

  it('rejects invalid IP addresses', () => {
    expect(validateIP('256.0.0.1').valid).toBe(false)
    expect(validateIP('192.168.1').valid).toBe(false)
    expect(validateIP('abc.def.ghi.jkl').valid).toBe(false)
  })

  it('rejects empty IP', () => {
    expect(validateIP('').valid).toBe(false)
    expect(validateIP(null).valid).toBe(false)
  })
})

describe('Cron Expression Validation', () => {
  it('validates correct cron expressions', () => {
    expect(validateCron('0 * * * *').valid).toBe(true)
    expect(validateCron('*/15 * * * *').valid).toBe(true)
    expect(validateCron('0 2 * * 0').valid).toBe(true)
  })

  it('rejects wrong number of fields', () => {
    expect(validateCron('* * *').valid).toBe(false)
    expect(validateCron('* * * * * *').valid).toBe(false)
  })

  it('rejects empty cron', () => {
    expect(validateCron('').valid).toBe(false)
    expect(validateCron(null).valid).toBe(false)
  })
})

describe('Form Validation Integration', () => {
  const validateForm = (fields) => {
    const errors = {}
    let isValid = true

    if (fields.email) {
      const result = validateEmail(fields.email.value)
      if (!result.valid) {
        errors.email = result.error
        isValid = false
      }
    }

    if (fields.password) {
      const result = validatePassword(fields.password.value)
      if (!result.valid) {
        errors.password = result.error
        isValid = false
      }
    }

    return { isValid, errors }
  }

  it('validates complete form', () => {
    const result = validateForm({
      email: { value: 'user@example.com' },
      password: { value: 'password123' },
    })

    expect(result.isValid).toBe(true)
    expect(Object.keys(result.errors).length).toBe(0)
  })

  it('returns multiple errors', () => {
    const result = validateForm({
      email: { value: 'invalid' },
      password: { value: '123' },
    })

    expect(result.isValid).toBe(false)
    expect(result.errors.email).toBeDefined()
    expect(result.errors.password).toBeDefined()
  })
})