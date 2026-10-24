import { categoryColor } from './categoryColor'

// Native <select> can't render colored dots inside <option>, so we show the
// selected value's dot next to the select and keep <option> as plain text.
export default function CategorySelect({ value, onChange, categories, required, placeholder = 'Category...' }) {
  return (
    <span className="category-select">
      {value && <span className="cat-dot" style={{ background: categoryColor(value) }} />}
      <select value={value} onChange={e => onChange(e.target.value)} required={required}>
        <option value="">{placeholder}</option>
        {categories.map(c => <option key={c.id} value={c.name}>{c.name}</option>)}
      </select>
    </span>
  )
}
