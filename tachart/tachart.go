package tachart

import (
	"errors"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/iamjinlei/go-tachart/charts"
	"github.com/iamjinlei/go-tachart/components"
	"github.com/iamjinlei/go-tachart/opts"
)

const (
	tooltipPositionFunc = `
		function(pos, params, el, elRect, size) {
			var obj = {top: 10};
			if (pos[0] > size.viewSize[0]/2) {
				obj['left'] = 30;
			} else {
				obj['right'] = 30;
			}
			return obj;
		}`
	tooltipFormatterFuncTpl = `
		function(value) {
			var eventMap = JSON.parse('__EVENT_MAP__');
			var title = (sz,txt) => '<span style="display:inline;line-height:'+(sz+2)+'px;font-size:'+sz+'px;font-weight:bold;">'+txt+'</span>';
			var square = (sz,sign,color,txt) => '<span style="display:inline;line-height:'+(sz+2)+'px;font-size:'+sz+'px;"><span style="display:inline-block;height:'+(sz+2)+'px;border-radius:3px;padding:1px 4px 1px 4px;text-align:center;margin-right:10px;background-color:' + color + ';vertical-align:top;">'+sign+'</span>'+txt+'</span>';
			var wrap = (sz,txt,width) => '<span style="display:inline-block;width:'+width+'px;word-break:break-word;word-wrap:break-word;white-space:pre-wrap;line-height:'+(sz+2)+'px;font-size:'+sz+'px;">'+txt+'</span>';

			value.sort((a, b) => a.seriesIndex -b.seriesIndex);
			var cdl = value[0];
			var ret = title(14, cdl.axisValueLabel)+ '  ['+cdl.dataIndex+']' + '<br/>' +
			square(13,'O',cdl.color,cdl.value[1].toFixed(__DECIMAL_PLACES__)) + '<br/>' +
			square(13,'C',cdl.color,cdl.value[2].toFixed(__DECIMAL_PLACES__)) + '<br/>' +
			square(13,'L',cdl.color,cdl.value[3].toFixed(__DECIMAL_PLACES__)) + '<br/>' +
			square(13,'H',cdl.color,cdl.value[4].toFixed(__DECIMAL_PLACES__)) + '<br/>';
			for (var i = 1; i < value.length; i++) {
				var s = value[i];
				ret += square(13,s.seriesName,s.color,s.value.toFixed(__DECIMAL_PLACES__)) + '<br/>';
			}

			var desc = eventMap[cdl.axisValueLabel];
			if (desc) {
				ret += '<hr>' + wrap(13,desc,160);
			}
			return ret;
		}`
	minRoundFuncTpl = `
		function(value) {
			return (value.min*0.99).toFixed(__DECIMAL_PLACES__);
		}`
	maxRoundFuncTpl = `
		function(value) {
			return (value.max*1.01).toFixed(__DECIMAL_PLACES__);
		}`
	yLabelFormatterFuncTpl = `
		function(value) {
			return value.toFixed(__DECIMAL_PLACES__);
		}`
)

var (
	ErrDuplicateCandleLabel = errors.New("candles with duplicated labels")

	// TODO: complete the map for all themes
	pageBgColorMap = map[Theme]string{
		ThemeWhite:   "#FFFFFF",
		ThemeVintage: "#FEF8EF",
	}
)

type gridLayout struct {
	top  int
	left int
	w    int
	h    int
}

type TAChart struct {
	// TODO: support dynamic auto-refresh
	cfg            Config
	globalOptsData globalOptsData
	extendedXAxis  []opts.XAxis
	extendedYAxis  []opts.YAxis
	gridLayouts    []gridLayout
}

func New(cfg Config) *TAChart {
	decimalPlaces := fmt.Sprintf("%v", cfg.precision)
	minRoundFunc := strings.Replace(minRoundFuncTpl, "__DECIMAL_PLACES__", decimalPlaces, -1)
	maxRoundFunc := strings.Replace(maxRoundFuncTpl, "__DECIMAL_PLACES__", decimalPlaces, -1)
	yLabelFormatterFunc := strings.Replace(yLabelFormatterFuncTpl, "__DECIMAL_PLACES__", decimalPlaces, -1)
	tooltipFormatterFunc := strings.Replace(tooltipFormatterFuncTpl, "__DECIMAL_PLACES__", decimalPlaces, -1)

	// grid layuout: N = len(indicators) + 1
	// ----------------------------------------
	//   candlestick chart + overlay + events (h/2)
	// ----------------------------------------
	//   		indicator chart               (h/2/N)
	//   			...
	//   		indicator chart               (h/2/N)
	// ----------------------------------------
	//   		  volume chart                (h/2/N)
	// ----------------------------------------

	left := 80
	right := 40
	sliderH := 70
	gap := 40

	h := (cfg.layout.chartHeight - sliderH) / (len(cfg.indicators) + 1 + 2)
	// candlestick+overlay
	cdlChartTop := 20
	// event
	eventChartTop := cdlChartTop + h*2 - 30
	eventChartH := 10

	grids := []opts.Grid{
		opts.Grid{ // candlestick + overlay
			Left:   px(left),
			Right:  px(right),
			Top:    px(cdlChartTop),
			Height: px(h * 2),
		},
		opts.Grid{ // event
			Left:   px(left),
			Right:  px(right),
			Top:    px(eventChartTop),
			Height: px(eventChartH),
		},
	}
	gridLayouts := []gridLayout{
		gridLayout{
			top:  cdlChartTop,
			left: left,
			w:    right - left,
			h:    h * 2,
		},
		gridLayout{
			top:  eventChartTop,
			left: left,
			w:    right - left,
			h:    eventChartH,
		},
	}
	xAxisIndex := []int{0, 1}
	extendedXAxis := []opts.XAxis{
		opts.XAxis{ // event
			Show:      false,
			GridIndex: 1,
		},
	}
	extendedYAxis := []opts.YAxis{
		opts.YAxis{ // event
			Show:      false,
			GridIndex: 1,
		},
	}

	// indicator & vol chart, inddex starting from 2
	top := cdlChartTop + h*2 + gap
	for i := 0; i < len(cfg.indicators)+1; i++ {
		gridIndex := i + 2
		grids = append(grids, opts.Grid{
			Left:   px(left),
			Right:  px(right),
			Top:    px(top),
			Height: px(h - gap),
		})
		gridLayouts = append(gridLayouts, gridLayout{
			top:  top,
			left: left,
			w:    right - left,
			h:    h - gap,
		})

		top += h

		xAxisIndex = append(xAxisIndex, gridIndex)

		extendedXAxis = append(extendedXAxis, opts.XAxis{
			Show:        true,
			GridIndex:   gridIndex,
			SplitNumber: 20,
			AxisTick: &opts.AxisTick{
				Show: false,
			},
			AxisLabel: &opts.AxisLabel{
				Show: false,
			},
		})
		// TODO: make this configurable
		min := minRoundFunc
		max := maxRoundFunc
		indYLabelFormatterFunc := yLabelFormatterFunc
		if i == len(cfg.indicators) {
			// volume
			min = "0"
			indYLabelFormatterFunc = strings.Replace(yLabelFormatterFuncTpl, "__DECIMAL_PLACES__", "0", -1)
		} else {
			v := cfg.indicators[i].yAxisLabel()
			if v != "" {
				indYLabelFormatterFunc = v
			}
			v = cfg.indicators[i].yAxisMin()
			if v != "" {
				min = v
			}
			v = cfg.indicators[i].yAxisMax()
			if v != "" {
				max = v
			}
		}

		extendedYAxis = append(extendedYAxis, opts.YAxis{
			Show:        true,
			GridIndex:   gridIndex,
			Scale:       true,
			SplitNumber: 2,
			SplitLine: &opts.SplitLine{
				Show: false,
			},
			AxisLabel: &opts.AxisLabel{
				Show:         true,
				ShowMinLabel: true,
				ShowMaxLabel: true,
				Formatter:    opts.FuncOpts(indYLabelFormatterFunc),
			},
			Min: opts.FuncOpts(min),
			Max: opts.FuncOpts(max),
		})
	}

	globalOptsData := globalOptsData{
		init: opts.Initialization{
			Theme:      string(cfg.theme),
			Width:      px(cfg.layout.chartWidth),
			Height:     px(cfg.layout.chartHeight),
			AssetsHost: cfg.assetsHost,
		},
		tooltip: opts.Tooltip{
			Show:      true,
			Trigger:   "axis",
			TriggerOn: "mousemove|click",
			Position:  opts.FuncOpts(tooltipPositionFunc),
			Formatter: opts.FuncOpts(tooltipFormatterFunc),
		},
		axisPointer: opts.AxisPointer{
			Type: "line",
			Snap: true,
			Link: opts.AxisPointerLink{
				XAxisIndex: "all",
			},
		},
		grids: grids,
		xAxis: opts.XAxis{ // candlestick+overlay
			Show:        true,
			GridIndex:   0,
			SplitNumber: 20,
		},
		yAxis: opts.YAxis{ // candlestick+overlay
			Show:      true,
			GridIndex: 0,
			Scale:     true,
			SplitArea: &opts.SplitArea{
				Show: true,
			},
			Min: opts.FuncOpts(minRoundFunc),
			Max: opts.FuncOpts(maxRoundFunc),
			AxisLabel: &opts.AxisLabel{
				Show:         true,
				ShowMinLabel: true,
				ShowMaxLabel: true,
				Formatter:    opts.FuncOpts(yLabelFormatterFunc),
			},
		},
		dataZoom: opts.DataZoom{
			Start:      50,
			End:        100,
			XAxisIndex: xAxisIndex,
		},
	}

	layout := gridLayouts[0]
	top = layout.top - 5
	for i, ol := range cfg.overlays {
		globalOptsData.titles = append(globalOptsData.titles, ol.getTitleOpts(top, layout.left+5, lineColors[i])...)
		top += chartLabelFontHeight
	}
	for i, ind := range cfg.indicators {
		layout := gridLayouts[i+2]
		globalOptsData.titles = append(globalOptsData.titles, ind.getTitleOpts(layout.top-5, layout.left+5, lineColors[i])...)
	}
	layout = gridLayouts[len(gridLayouts)-1]
	globalOptsData.titles = append(globalOptsData.titles, opts.Title{
		TitleStyle: &opts.TextStyle{
			FontSize: chartLabelFontSize,
		},
		Title: "Vol",
		Left:  px(layout.left + 5),
		Top:   px(layout.top - 5),
	})

	return &TAChart{
		cfg:            cfg,
		globalOptsData: globalOptsData,
		extendedXAxis:  extendedXAxis,
		extendedYAxis:  extendedYAxis,
		gridLayouts:    gridLayouts,
	}
}

func (c TAChart) GenStatic(cdls []Candle, events []Event, path string) error {
	xAxis := make([]string, 0)
	klineSeries := []opts.KlineData{}
	volSeries := []opts.BarData{}
	closes := []float64{}
	cdlMap := map[string]*Candle{}
	for _, cdl := range cdls {
		xAxis = append(xAxis, cdl.Label)
		// open,close,low,high
		klineSeries = append(klineSeries, opts.KlineData{Value: []float64{cdl.O, cdl.C, cdl.L, cdl.H}})
		closes = append(closes, cdl.C)

		style := &opts.ItemStyle{
			Color:   colorUpBar,
			Opacity: opacity,
		}
		if cdl.O > cdl.C {
			style = &opts.ItemStyle{
				Color:   colorDownBar,
				Opacity: opacity,
			}
		}
		volSeries = append(volSeries, opts.BarData{
			Value:     cdl.V,
			ItemStyle: style,
		})

		if cdlMap[cdl.Label] != nil {
			return ErrDuplicateCandleLabel
		}
		c := cdl
		cdlMap[cdl.Label] = &c
	}

	// candlestick+overlay
	chart := charts.NewKLine().SetXAxis(xAxis).AddSeries("kline",
		klineSeries,
		charts.WithKlineChartOpts(opts.KlineChart{
			BarWidth:   "60%",
			XAxisIndex: 0,
			YAxisIndex: 0,
		}),
		charts.WithItemStyleOpts(opts.ItemStyle{
			Color:        colorUpBar,
			Color0:       colorDownBar,
			BorderColor:  colorUpBar,
			BorderColor0: colorDownBar,
			Opacity:      opacity,
		}),
	)

	eventDescMap := map[string]string{}
	for _, e := range events {
		eventDescMap[e.Label] = e.Description
	}

	chart.SetGlobalOptions(c.globalOptsData.genOpts(eventDescMap)...)

	for i, ol := range c.cfg.overlays {
		chart.Overlap(ol.genChart(closes, xAxis, 0, lineColors[i]))
	}

	for i := 0; i < len(c.extendedXAxis); i++ {
		c.extendedXAxis[i].Data = xAxis
	}
	chart.ExtendXAxis(c.extendedXAxis...)
	chart.ExtendYAxis(c.extendedYAxis...)

	evtOpts := []charts.SeriesOpts{
		charts.WithBarChartOpts(opts.BarChart{
			BarWidth:   "60%",
			XAxisIndex: 1,
			YAxisIndex: 1,
		}),
	}
	for _, e := range events {
		evtOpts = append(evtOpts, charts.WithMarkPointNameCoordItemOpts(opts.MarkPointNameCoordItem{
			Symbol:     "roundRect",
			SymbolSize: 16,
			Coordinate: []interface{}{e.Label, 0},
			Label:      eventLabelMap[e.Type].label,
			ItemStyle:  eventLabelMap[e.Type].style,
		}))
	}
	event := charts.NewBar().AddSeries("events", []opts.BarData{}, evtOpts...)
	chart.Overlap(event)

	// grid index starting from 2 (candlestick+event)
	for i, ind := range c.cfg.indicators {
		chart.Overlap(ind.genChart(closes, xAxis, i+2, ""))
	}

	bar := charts.NewBar().
		SetXAxis(xAxis).
		AddSeries("Vol", volSeries, charts.WithBarChartOpts(opts.BarChart{
			BarWidth:   "60%",
			XAxisIndex: len(c.cfg.indicators) + 2,
			YAxisIndex: len(c.cfg.indicators) + 2,
		}))
	chart.Overlap(bar)
	chart.AddJSFuncs(c.cfg.jsFuncs...)

	fp, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fp.Close()

	layout := components.Layout{
		TemplateColumns: template.CSS(fmt.Sprintf("%vpx %vpx %vpx", c.cfg.layout.leftWidth, c.cfg.layout.chartWidth, c.cfg.layout.rightWidth)),
		TopHeight:       template.CSS(px(c.cfg.layout.topHeight)),
		BottomHeight:    template.CSS(px(c.cfg.layout.bottomHeight)),
		TopContent:      template.HTML(c.cfg.layout.topContent),
		BottomContent:   template.HTML(c.cfg.layout.bottomContent),
		LeftContent:     template.HTML(c.cfg.layout.leftContent),
		RightContent:    template.HTML(c.cfg.layout.rightContent),
	}

	pageBgColor := pageBgColorMap[c.cfg.theme]
	if pageBgColor == "" {
		pageBgColor = "#FFFFFF"
	}

	return components.NewPage(c.cfg.assetsHost).
		SetLayout(layout).
		SetBackgroundColor(pageBgColor).
		AddCharts(chart).
		Render(fp)
}

func px(v int) string {
	return fmt.Sprintf("%vpx", v)
}
