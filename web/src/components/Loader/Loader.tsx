import './Loader.css'

type LoaderProps = { darkMode: boolean }

export const Loader = (props: LoaderProps) => {
	return (
		<div className={'loader' + (props.darkMode ? ' loader-dark' : '')}>
			<div></div>
			<div></div>
			<div></div>
			<div></div>
		</div>
	)
}
