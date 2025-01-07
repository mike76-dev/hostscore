import './HostResults.css'
import { useState, useEffect } from 'react'
import {
	Host,
	HostBenchmark,
	useLocations,
	convertSize
} from '../../api'
import { Benchmark, Tooltip } from '../'

type HostResultsProps = {
	darkMode: boolean,
	host: Host,
	node: string
}

type Results = {
	node: string,
	scanCount: number,
	latency: number,
	benchmarkCount: number,
	upload: number,
	download: number,
	ttfb: number,
	data: HostBenchmark[]
}

const TTFBTooltip = () => (
	<div>
		This is not the Time To First Byte as it is usually meant,
		but rather the time to download the first data sector
		(4 MiB) from the host.
	</div>
)

export const HostResults = (props: HostResultsProps) => {
	const locations = useLocations()
	const [results, setResults] = useState<Results[]>([])
	const [benchmarkData, setBenchmarkData] = useState<(HostBenchmark | undefined)[][]>([])
	useEffect(() => {
		let res: Results[] = []
		let bd: (HostBenchmark | undefined)[][] = []
		let rows = 0
		if (props.node === 'global') {
			locations.forEach(location => {
				res.push({
					node: location.short,
					scanCount: 0,
					latency: 0,
					benchmarkCount: 0,
					upload: 0,
					download: 0,
					ttfb: 0,
					data: []
				})
			})
		} else {
			res.push({
				node: props.node,
				scanCount: 0,
				latency: 0,
				benchmarkCount: 0,
				upload: 0,
				download: 0,
				ttfb: 0,
				data: []
			})
		}
		locations.forEach(location => {
			if (!props.host.interactions || (props.node !== 'global' && props.node !== location.short)) return
			const interactions = props.host.interactions[location.short]
			if (!interactions) return
			const index = res.findIndex(r => r.node === location.short)
			if (index < 0) return
			interactions.scanHistory?.forEach(scan => {
				if (scan.success) {
					res[index].latency += scan.latency
					res[index].scanCount++
				}
			})
			interactions.benchmarkHistory?.forEach(benchmark => {
				if (benchmark.success) {
					res[index].upload += benchmark.uploadSpeed
					res[index].download += benchmark.downloadSpeed
					res[index].ttfb += benchmark.ttfb
					res[index].benchmarkCount++
				}
				if (res[index].data.length < 12) {
					res[index].data.push(benchmark)
					if (res[index].data.length > rows) rows = res[index].data.length
				}
			})
		})
		for (let j = 0; j < rows; j++) {
			let row: (HostBenchmark | undefined)[] = []
			for (let i = 0; i < res.length; i++) {
				row.push(j >= res[i].data.length ? undefined : res[i].data[j])
			}
			bd.push(row)
		}
		res.forEach(r => {
			if (r.scanCount > 0) r.latency /= (r.scanCount * 1e6)
			if (r.benchmarkCount > 0) {
				r.upload /= r.benchmarkCount
				r.download /= r.benchmarkCount
				r.ttfb /= (r.benchmarkCount * 1e9)
			}
		})
		setResults(res)
		setBenchmarkData(bd)
	}, [props.host, props.node, locations])
	return (
		<div className={'host-results-container' + (props.darkMode ? ' host-results-dark' : '')}>
			<table className="host-results-table">
				<thead>
					<tr>
						<th></th>
						{results.map(res => (
							<th key={'header-' + res.node}>
								{locations.find(loc => res.node === loc.short)?.long || ''}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					<tr>
						<td>Latency</td>
						{results.map(res => (
							<td key={'latency-' + res.node}>
								{res.scanCount > 0 ? res.latency.toFixed(0) + ' ms' : 'N/A'}
							</td>
						))}
					</tr>
					<tr>
						<td>Upload Speed</td>
						{results.map(res => (
							<td key={'upload-' + res.node}>
								{res.benchmarkCount > 0 ? convertSize(res.upload) + '/s' : 'N/A'}
							</td>
						))}
					</tr>
					<tr>
						<td>Download Speed</td>
						{results.map(res => (
							<td key={'download-' + res.node}>
								{res.benchmarkCount > 0 ? convertSize(res.download) + '/s' : 'N/A'}
							</td>
						))}
					</tr>
					<tr>
						<td>
							TTFB
							<Tooltip className="host-results-tooltip" darkMode={props.darkMode}>
								<TTFBTooltip/>
							</Tooltip>
						</td>
						{results.map(res => (
							<td key={'ttfb-' + res.node}>
								{res.benchmarkCount > 0 ? res.ttfb.toFixed(2) + ' s' : 'N/A'}
							</td>
						))}
					</tr>
				</tbody>
			</table>
			{props.host && benchmarkData.length > 0 &&
				<table className="host-benchmarks-table">
					<thead>
						<tr>
							{results.map(res => (
								<th key={'benchmark-header-' + res.node}>
									{locations.find(loc => res.node === loc.short)?.long || ''}
								</th>
							))}
						</tr>
					</thead>
					<tbody>
						{benchmarkData.map((row, j) => (
							<tr key={'benchmark-row-' + j}>
								{row.map((cell, i) => (
									<td key={'benchmark-' + j + '-' + i} style={{width: '' + 100/benchmarkData.length + '%'}}>
										{cell &&
											<Benchmark
												timestamp={cell.timestamp}
												success={cell.success}
												upload={cell.uploadSpeed}
												download={cell.downloadSpeed}
												error={cell.error}
											/>
										}
									</td>
								))}
							</tr>
						))}
					</tbody>
				</table>
			}
		</div>
	)
}