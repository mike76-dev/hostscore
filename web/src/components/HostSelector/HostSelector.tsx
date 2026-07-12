import './HostSelector.css'

type HostSelectorProps = {
	value: string,
	onChange: (value: string) => any,
	darkMode: boolean
}

export const HostSelector = (props: HostSelectorProps) => {
	return (
		<div className="seg" role="group" aria-label="Host set">
			<button
				tabIndex={1}
				aria-pressed={props.value === 'online'}
				onClick={() => props.onChange('online')}
			>Online</button>
			<button
				tabIndex={1}
				aria-pressed={props.value === 'all'}
				onClick={() => props.onChange('all')}
			>All</button>
		</div>
	)
}
