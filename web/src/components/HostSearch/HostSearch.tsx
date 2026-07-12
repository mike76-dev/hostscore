import './HostSearch.css'

type HostSearchProps = {
	darkMode: boolean,
	value: string,
	onChange: (value: string) => any
}

export const HostSearch = (props: HostSearchProps) => {
	return (
		<label className="host-search-box">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
				<circle cx="11" cy="11" r="7"/>
				<path d="m20 20-3.5-3.5"/>
			</svg>
			<input
				className="host-search-input"
				type="search"
				placeholder="Search net address…"
				value={props.value}
				onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
					props.onChange(event.target.value)
				}}
				tabIndex={1}
			/>
		</label>
	)
}
