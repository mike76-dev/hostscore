import './CountrySelector.css'
import { countryByCode } from '../../api'

type CountrySelectorProps = {
	options: string[],
	value: string,
	onChange: (value: string) => any,
	darkMode: boolean
}

export const CountrySelector = (props: CountrySelectorProps) => {
	return (
		<select
			className="ctl"
			aria-label="Filter by country"
			value={props.value}
			onChange={(event: React.ChangeEvent<HTMLSelectElement>) => {
				props.onChange(event.target.value)
			}}
			tabIndex={1}
		>
			<option key="country-none" value="">All countries</option>
			{props.options && props.options
				.sort((a, b) => (countryByCode(a) || '').localeCompare(countryByCode(b) || ''))
				.map(option => (
				<option
					key={'country-' + option}
					value={option}
				>{countryByCode(option)}</option>
			))}
		</select>
	)
}
