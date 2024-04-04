import './Loader.css'

type LoaderProps = {
    darkMode: boolean,
    className?: string
}

export const Loader = (props: LoaderProps) => {
	return (
		<div className={'loader' + (props.darkMode ? ' loader-dark ' : ' ') + (props.className || '')}>
			<div></div>
			<div></div>
			<div></div>
			<div></div>
		</div>
	)
}
