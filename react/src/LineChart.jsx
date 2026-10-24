import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend } from 'chart.js'
import { Line } from 'react-chartjs-2'
import { formatRupiah } from './api'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

const isDark = () => document.documentElement.getAttribute('data-theme') === 'dark'
const cssVar = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()
const resolveColor = (c) => (c.startsWith('var(') ? cssVar(c.slice(4, -1)) : c)

/**
 * Line chart, built on Chart.js.
 * datasets: [{ label, data: number[], color }]
 * labels: string[] (x-axis categories)
 */
export default function LineChart({ labels, datasets, height = 260 }) {
  const textColor = isDark() ? '#e5e7eb' : '#1e2330'
  const gridColor = isDark() ? '#2c3350' : '#e5e7eb'

  const data = {
    labels,
    datasets: datasets.map(d => ({
      label: d.label,
      data: d.data,
      borderColor: resolveColor(d.color),
      backgroundColor: resolveColor(d.color),
      tension: 0.3,
      pointRadius: 0,
      pointHoverRadius: 4,
      borderWidth: 2,
    })),
  }

  const options = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: 'nearest', axis: 'x', intersect: false },
    plugins: {
      legend: { display: datasets.length > 1, labels: { color: textColor, boxWidth: 10, font: { size: 11 } } },
      tooltip: {
        callbacks: {
          label: (ctx) => `${ctx.dataset.label}: ${formatRupiah(ctx.raw)}`,
        },
      },
      datalabels: { display: false },
    },
    scales: {
      x: { ticks: { color: textColor, font: { size: 11 } }, grid: { display: false } },
      y: { ticks: { color: textColor, font: { size: 10 }, callback: (v) => formatRupiah(v) }, grid: { color: gridColor } },
    },
  }

  return <div style={{ height }}><Line data={data} options={options} /></div>
}
