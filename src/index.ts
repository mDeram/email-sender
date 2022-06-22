import nodemailer from "nodemailer";
import "dotenv/config";
import amqp from "amqplib";

const ___prod___ = process.env.NODE_ENV === "production";
const QUEUE_NAME = "email";

interface Email {
    from: string;
    to: string;
    subject: string;
    text: string;
    html: string;
}

async function main() {
    const connection = await amqp.connect("amqp://guest:guest@localhost:5672/");
    const channel = await connection.createChannel();

    await channel.assertQueue(QUEUE_NAME);

    channel.consume(QUEUE_NAME, async msg => {
        if (!msg) return;

        const email = JSON.parse(msg.content.toString()) as Email;
        try {
            await sendMail(email);
            channel.ack(msg);
        } catch(e) {
            console.error("could not send the email or acknowledge", e);
        }
    });
}

async function sendMail(email: Email) {
    const test = !___prod___ && email.to.endsWith("@test.com");

    // Transporter could be initialized only once for performances
    const transporter = nodemailer.createTransport({
        host: "127.0.0.1",
        port: test ? 7777 : 1025,
        secure: false,
        auth: {
            user: test ? undefined : process.env.MAIL_USER,
            pass: test ? undefined : process.env.MAIL_PASS,
        },
        tls: {
            rejectUnauthorized: false
        }
    });

    const info = await transporter.sendMail(email);
    console.log("Message sent: %s", info.messageId);
}

main().catch(console.error);
