import { useState, useEffect } from 'react'

function formatThousands(n) {
  if (n === '' || n === null || n === undefined) return ''
  const num = Number(n)
  if (Number.isNaN(num)) return ''
  return num.toLocaleString('id-ID')
}

function parseThousands(s) {
  const digits = String(s).replace(/[^\d-]/g, '')
  return digits === '' ? '' : Number(digits)
}

/** Number input that auto-formats with thousands separators (id-ID: 1.000.000) while typing. */
export default function MoneyInput({ value, onChange, onBlur, ...props }) {
  const [display, setDisplay] = useState(formatThousands(value))

  useEffect(() => { setDisplay(formatThousands(value)) }, [value])

  function handleChange(e) {
    const raw = parseThousands(e.target.value)
    setDisplay(formatThousands(raw))
    onChange && onChange(raw === '' ? '' : raw)
  }

  return (
    <input
      {...props}
      type="text"
      inputMode="numeric"
      value={display}
      onChange={handleChange}
      onBlur={(e) => onBlur && onBlur(parseThousands(e.target.value))}
    />
  )
}
