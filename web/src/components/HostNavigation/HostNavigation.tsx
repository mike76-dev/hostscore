import './HostNavigation.css'
import React from 'react'

type HostNavigationProps = {
	darkMode: boolean,
	offset: number,
	limit: number,
	total: number,
	changeOffset: (offset: number) => any,
	changeLimit: (limit: number) => any
}

export const HostNavigation = (props: HostNavigationProps) => {
	const first = () => {props.changeOffset(0)}
	const prev = () => {props.changeOffset(props.offset - props.limit)}
	const next = () => {props.changeOffset(props.offset + props.limit)}
	const last = () => {props.changeOffset(props.limit * Math.floor(props.total / props.limit))}
	const goto = (event: React.ChangeEvent<HTMLInputElement>) => {
		let oldValue = Math.floor(props.offset / props.limit) + 1
		let newValue = Number.parseInt(event.target.value, 10)
		if (newValue > 0 && Number.isInteger(newValue) && newValue <= props.limit * Math.floor(props.total / props.limit) + 1) {
			props.changeOffset((newValue - 1) * props.limit)
		} else {
			event.target.value = '' + oldValue
		}
	}
	const changeRows = (event: React.ChangeEvent<HTMLSelectElement>) => {
		let newLimit = Number.parseInt(event.target.value)
		props.changeLimit(newLimit)
	}
	return (
		<div className={'host-navigation-container' + (props.darkMode ? ' host-navigation-dark' : '')}>
			<label>
				<span className="host-navigation-text">Rows to display:</span>
				<select
					className="host-navigation-select"
					tabIndex={1}
					value={props.limit}
					onChange={changeRows}
				>
					<option value="10">10</option>
					<option value="20">20</option>
					<option value="50">50</option>
				</select>
			</label>
			<button
				className="host-navigation-button"
				tabIndex={1}
				disabled={props.offset === 0}
				onClick={first}
			>&lt;&lt;</button>
			<button
				className="host-navigation-button"
				tabIndex={1}
				disabled={props.offset === 0}
				onClick={prev}
			>&lt;</button>
			<input
				className="host-navigation-input"
				type="number"
				tabIndex={1}
				value={Math.floor(props.offset / props.limit) + 1}
				onChange={goto}
			/>
			<span className="host-navigation-total">/ {Math.ceil(props.total / props.limit)}</span>
			<button
				className="host-navigation-button"
				tabIndex={1}
				disabled={props.offset + props.limit >= props.total}
				onClick={next}
			>&gt;</button>
			<button
				className="host-navigation-button"
				tabIndex={1}
				disabled={props.offset + props.limit >= props.total}
				onClick={last}
			>&gt;&gt;</button>
		</div>
	)
}
