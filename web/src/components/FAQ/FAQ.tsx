import './FAQ.css'
import { useState, useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, FAQItem } from '../'
import Back from '../../assets/back.png'
import { NetworkContext } from '../../contexts'

type Topic = {
    question: string,
    answer?: React.ReactNode,
    subtopics?: Topic[]
}

const topics: Topic[] = [
    {
        question: 'How is the score calculated?',
        answer: <>
            <p>
                The score consists of ten metrics, each of which can take any values
                from 0 to 1. Seven of these metrics match those used by the host scoring
                algorithm of <code>renterd</code>: Prices, Remaining Storage, Collateral,
                Interactions, Uptime, Age, and Version. HostScore introduced three more
                metrics: Accepting Contracts, Latency, and Benchmarks. The total score
                is the product of all ten metrics, so it also ranges from 0 to 1.
            </p>
            <p>
                Each benchmarking node keeps its own scoring of the hosts. You can view
                the scores per node or averaged.
            </p>
            <p>
                The rank of a host is inversely related to the average total score.
                The higher the score, the lower the rank.
            </p>
        </>,
        subtopics: [
            {
                question: 'Prices',
                answer: <>
                    <p>
                        This is the measure of how expensive it is to store a given
                        amount of data for a given period of time on the host.
                    </p>
                    <p>
                        If the costs exactly match the expectations, the score is 0.5.
                    </p>
                    <p>
                        If the host is cheaper than expected, a linear bonus is applied.
                        The best score of 1 is reached when the ratio between the costs
                        and the expectations is 10x.
                    </p>
                    <p>
                        If the host is more expensive than expected, an exponential malus
                        is applied. A 2x ratio will already cause the score to drop to 0.16,
                        and a 3x ratio causes it to drop to 0.05.
                    </p>
                    <p>
                        For the purpose of scoring all hosts under exactly the same
                        conditions, HostScore assumes storing 1 TiB of data for one month
                        for the price of 1 KS.
                    </p>
                </>
            },
            {
                question: 'Remaining Storage',
                answer: <>
                    <p>
                        This metric reflects how satisfied a renter would be with the
                        amount of available storage on the host.
                    </p>
                    <p>
                        For the purpose of scoring all hosts under exactly the same
                        conditions, HostScore assumes storing 1 TiB of data and expects
                        to occupy up to 25% of the host's remaining storage.
                    </p>
                    <p>
                        The score for the host is the square of the amount of storage we
	                    expected divided by the amount of storage we want. If we expect
                        to be able to store more data on the host than we need to allocate,
                        the host gets full score for storage. Otherwise, the score of the
                        host is the fraction of the data we expect raised to the storage
                        penalty exponentiation.
                    </p>
                </>
            },
            {
                question: 'Collateral',
                answer: <>
                    <p>
                        This is the measure of the host's collateral relative to its
                        storage price.
                    </p>
                    <p>
                        The collateral score is a linear function between 0 and 1, where
                        the lower limit is 1.5x the storage price, and the upper limit is
                        6x the storage price. Beyond that, there is no effect on the score.
                    </p>
                </>
            },
            {
                question: 'Interactions',
                answer: <>
                    <p>
                        This metric reflects the number of historic successful interactions
                        with the host relative to the total number of interactions. 
                    </p>
                    <p>
                        Each successful scan or benchmark adds 1 to the successful interactions,
                        while each failed scan or benchmark adds 1 to the failed ones.
                        This is a function that starts at 0.72 with an empty interactions
                        history, but the penalty for the failed interactions is much greater
                        than the bonus for the successful interactions. For example, if your
                        host had 10 successful interactions out of 10, the score will be 0.78.
                        With 100 successful interactions out of 100, it will become 0.93.
                        However, with 10 failed interactions out of 10, the score will be 0.04,
                        and with 100 failed interactions out of 100, it will drop to 4e-7.
                    </p>
                </>
            },
            {
                question: 'Uptime',
                answer: <>
                    <p>
                        The host scans run every 30 minutes. Each successful scan adds 30 minutes
                        to the host's total uptime, while each failed scan adds 30 minutes to
                        the total downtime. Uptime score is the measure of the total uptime
                        related to the total downtime.
                    </p>
                    <p>
                        Up to 2% of downtime is forgiven unconditionally. This means that
                        a host with 98% uptime or more receives a score of 1. On the other
                        hand, poor uptime reduces the score exponentially. So, a host with
                        95% uptime will receive a score of 0.6, while 90% uptime will bring
                        the score down to 0.12.
                    </p>
                </>
            },
            {
                question: 'Age',
                answer: <>
                    <p>
                        This metric introduces an underage penalty for the new hosts.
                    </p>
                    <p>
                        A brand new host will receive an age score of 0.001. The older the
                        host grows, the lower the penalty gets. After 8 days, the score
                        improves to 0.08. After one month, it becomes 0.33. After 128 days,
                        the penalty goes away completely.
                    </p>
                </>
            },
            {
                question: 'Version',
                answer: <>
                    <p>
                        This metric brings in a penalty for the hosts that haven't upgraded
                        to the latest version.
                    </p>
                    <p>
                        Currently, the newest hosting software is <code>hostd</code>,
                        which is equal to the version 1.6.0. <code>siad</code> hosts
                        running the 1.5.9 version receive a version score of 0.99. All
                        earlier versions automatically get a score of 0.
                    </p>
                    <p>
                        In the future, the scoring algorithm will probably be modified
                        to differentiate between the releases of <code>hostd</code>.
                    </p>
                </>
            },
            {
                question: 'Accepting Contracts',
                answer: <>
                    <p>
                        This metric is quite straightforward. If a host is accepting
                        new contracts, it receives a score of 1. Otherwise, the score is
                        zero.
                    </p>
                </>
            },
            {
                question: 'Latency',
                answer: <>
                    <p>
                        This is the measure of the host's latency, i.e. how quickly a
                        host responds to the scans.
                    </p>
                    <p>
                        Latency score is a linear function between 0 and 1, where the lower
                        limit is the latencies of 1 second and greater, and the upper
                        limit is the latencies of 10 milliseconds or less.
                    </p>
                </>
            },
            {
                question: 'Benchmarks',
                answer: <>
                    <p>
                        This is a combined measure of the upload and download speeds
                        taken by a benchmarking node.
                    </p>
                    <p>
                        Both the upload and the download component are linear functions
                        between 0 and 1, where the lower limit represents the speeds of
                        1 MB/s and lower, and the upper limit represents the speeds of
                        50 MB/s (for uploads) and 100 MB/s (for downloads). The resulting
                        score is the product of both components.
                    </p>
                </>
            }
        ]
    },
    {
        question: 'How often are the benchmarks run?',
        answer: <>
            <p>
                There are two types of interactions between HostScore's benchmarking nodes
                and the hosts: the scans and the benchmarks.
            </p>
            <p>
                The scans are run every 30 minutes. During a scan, the host's settings and
                the host's current price table are retrieved, and the latency is measured.
                The scans also determine whether a host is online or offline.
            </p>
            <p>
                If the host is offline for a long time, the scan frequency is reduced.
                However, each host on the network is scanned at least once in 24 hours.
            </p>
            <p>
                During a benchmark, 64 MiB of data is uploaded to and downloaded from the host.
                The nodes target to benchmark the hosts every 2 hours. In practice, though,
                the benchmark intervals are longer. This is because, while there can be many
                host scans run at a time, there can only be one benchmark run at a time by
                each node, to minimize the error of the measurement.
            </p>
            <p>
                If the host has failed several benchmarks in a row, the benchmarking
                frequency is reduced. The algorithm picking the hosts for benchmarking
                makes sure that the hosts that have been offline for too long are not
                benchmarked at all.
            </p>
        </>
    },
    {
        question: `Why are my host's latencies and speeds not changing over time?`,
        answer: <>
            <p>
                The latencies and the speeds shown in the host's details are averaged
                over a relatively large number of scans (48) and benchmarks (12).
                So, they may indeed seem static if the host's performance is consistent.
            </p>
        </>
    },
    {
        question: 'Do the average prices include the 3x redundancy?',
        answer: <>
            <p>
                The network average prices are shown from the hosts' perspective.
                They don't include any redundancy.
            </p>
        </>
    },
    {
        question: `I have a question but it's not listed here. What shall I do?`,
        answer: <>
            <p>Please let me know, and I will consider listing your question here.</p>
        </>
    }
]

type FAQProps = { darkMode: boolean }

export const FAQ = (props: FAQProps) => {
    const navigate = useNavigate()
    const { network } = useContext(NetworkContext)
    const [expandedItem, setExpandedItem] = useState(0)
    const expandItem = (parent: number, child: number) => {
        if (child !== 0) setExpandedItem(child)
        else setExpandedItem(parent)
    }
    return (
        <div className={'faq-container' + (props.darkMode ? ' faq-container-dark' : '')}>
            <h1>Frequently Asked Questions</h1>
            {topics.map((topic, index) => (
                <FAQItem
                    key={'faq-' + index}
                    parent={0}
                    index={index + 1}
                    title={topic.question}
                    expanded={expandedItem === index + 1 || (expandedItem > 100 && Math.floor(expandedItem / 100) - 1 === index)}
                    expandItem={expandItem}
                >
                    {topic.answer}
                    {topic.subtopics && topic.subtopics.map((subtopic, i) => (
                        <FAQItem
                            key={`faq-` + index + '-' + i}
                            parent={index + 1}
                            index={100 * (index + 1) + i + 1}
                            title={subtopic.question}
                            expanded={expandedItem > 100 && Math.floor(expandedItem / 100) - 1 === index && expandedItem % 100 - 1 === i}
                            expandItem={expandItem}
                        >
                            {subtopic.answer}
                        </FAQItem>
                    ))}
                </FAQItem>
            ))}
            <Button
				icon={Back}
				caption="back"
				darkMode={props.darkMode}
				onClick={() => {navigate(network === 'zen' ? '/zen' : '/')}}
			/>
        </div>
    )
}