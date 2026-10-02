// The ECharts build the app draws with (UI U8): echarts/core plus only the
// chart types and components UiChart offers, so the bundler drops the rest
// (maps, 3D, the other series). UiChart imports this module on first use,
// which keeps ECharts out of the entry chunk and out of pages without a
// chart; vite.config.js puts echarts and zrender in the `echarts` chunk.
// Adding a series type: import it here and list it in use().
import { init, registerTheme, use } from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

use([LineChart, BarChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

export { init, registerTheme }
