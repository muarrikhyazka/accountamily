import { createContext, useContext, useState } from 'react'

const now = new Date()
const YearContext = createContext(null)

export function YearProvider({ children }) {
  const [year, setYear] = useState(now.getFullYear())
  const [month, setMonth] = useState(now.getMonth() + 1)
  const [valueMax, setValueMax] = useState('all') // 'all' | '10jt' | '100jt'
  return (
    <YearContext.Provider value={{ year, setYear, month, setMonth, valueMax, setValueMax }}>
      {children}
    </YearContext.Provider>
  )
}

export function useYearMonth() {
  return useContext(YearContext)
}
