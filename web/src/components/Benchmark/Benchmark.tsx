import './Benchmark.css'

type BenchmarkProps = {
	timestamp: string,
	success: boolean,
	error: string
}

export const Benchmark = (props: BenchmarkProps) => {
	return (
		<div className="benchmark-container">
			<div className={'benchmark-' + (props.success ? 'pass' : 'fail')}>
				{new Date(props.timestamp).toLocaleString()}
			</div>
			{props.success === false ?
				<div className="benchmark-error">{props.error}</div>
			: <></>
			}
		</div>
	)
}